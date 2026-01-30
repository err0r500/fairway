package fairway

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// Job represents a queued automation job
type Job struct {
	Key       fdb.Key
	EventVS   dcb.Versionstamp // versionstamp of the event to process
	VestingNs int64            // when job becomes available (unix nano)
	ExpiryNs  int64            // when lease expires (unix nano)
	LeaseVS   dcb.Versionstamp // hybrid clock for lease expiry check
	OwnerID   [16]byte         // worker that owns this job
	Attempts  uint8            // number of attempts so far
}

var (
	ErrNoJobs     = errors.New("no jobs available")
	ErrLeaseStolen = errors.New("lease was stolen by another worker")
)

// Job value format (47 bytes total):
// [vesting_ns:8][expiry_ns:8][lease_vs:12][owner_id:16][attempts:1]
const jobValueSize = 8 + 8 + 12 + 16 + 1 // 45 bytes

func encodeJob(j *Job) []byte {
	buf := make([]byte, jobValueSize)
	binary.BigEndian.PutUint64(buf[0:8], uint64(j.VestingNs))
	binary.BigEndian.PutUint64(buf[8:16], uint64(j.ExpiryNs))
	copy(buf[16:28], j.LeaseVS[:])
	copy(buf[28:44], j.OwnerID[:])
	buf[44] = j.Attempts
	return buf
}

func decodeJob(key fdb.Key, value []byte) (*Job, error) {
	if len(value) != jobValueSize {
		return nil, errors.New("invalid job value size")
	}
	j := &Job{
		Key:       key,
		VestingNs: int64(binary.BigEndian.Uint64(value[0:8])),
		ExpiryNs:  int64(binary.BigEndian.Uint64(value[8:16])),
		Attempts:  value[44],
	}
	copy(j.LeaseVS[:], value[16:28])
	copy(j.OwnerID[:], value[28:44])
	return j, nil
}

// extractEventVSFromJobKey extracts the event versionstamp from a job key
// Job key format: queue.Pack(tuple.Tuple{eventVS, rand20})
func extractEventVSFromJobKey(queueDir subspace.Subspace, key fdb.Key) (dcb.Versionstamp, error) {
	keyTuple, err := queueDir.Unpack(key)
	if err != nil {
		return dcb.Versionstamp{}, err
	}
	if len(keyTuple) < 1 {
		return dcb.Versionstamp{}, errors.New("invalid job key: missing event versionstamp")
	}

	tupleVs, ok := keyTuple[0].(tuple.Versionstamp)
	if !ok {
		return dcb.Versionstamp{}, errors.New("invalid job key: first element not a versionstamp")
	}

	var vs dcb.Versionstamp
	copy(vs[:10], tupleVs.TransactionVersion[:])
	binary.BigEndian.PutUint16(vs[10:12], tupleVs.UserVersion)
	return vs, nil
}

// enqueueInTx enqueues a job for the given event versionstamp
func (a *Automation[Deps]) enqueueInTx(tr fdb.Transaction, eventVS dcb.Versionstamp) error {
	// Convert dcb.Versionstamp to tuple.Versionstamp
	var txVersion [10]byte
	copy(txVersion[:], eventVS[:10])
	userVersion := binary.BigEndian.Uint16(eventVS[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	// Generate random suffix for uniqueness
	var rand20 [20]byte
	if _, err := rand.Read(rand20[:]); err != nil {
		return err
	}

	// Job key: queue/<eventVS>/<rand20>
	jobKey := a.queueDir.Pack(tuple.Tuple{tupleVs, rand20[:]})

	// Job value: metadata only, event fetched from dcb when processing
	job := &Job{
		VestingNs: 0, // available immediately
		ExpiryNs:  0, // no lease yet
		Attempts:  0,
	}

	tr.Set(jobKey, encodeJob(job))
	return nil
}

// dequeue attempts to claim a job from the queue
func (a *Automation[Deps]) dequeue() (*Job, error) {
	var job *Job

	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		now := time.Now().UnixNano()

		// Range read from queue
		iter := tr.GetRange(a.queueDir, fdb.RangeOptions{
			Limit: a.config.BatchSize,
		}).Iterator()

		for iter.Advance() {
			kv, err := iter.Get()
			if err != nil {
				return nil, err
			}

			j, err := decodeJob(kv.Key, kv.Value)
			if err != nil {
				continue // skip malformed jobs
			}

			// Check if job is vested (available)
			if j.VestingNs > now {
				continue
			}

			// Check if job is owned and lease not expired
			if j.OwnerID != [16]byte{} && j.ExpiryNs > now {
				continue
			}

			// Extract event VS from key
			eventVS, err := extractEventVSFromJobKey(a.queueDir, kv.Key)
			if err != nil {
				continue
			}
			j.EventVS = eventVS

			// Claim the job
			j.OwnerID = a.workerID
			j.ExpiryNs = now + int64(a.config.LeaseTTL)
			// LeaseVS would ideally use FDB versionstamp but for simplicity use timestamp
			binary.BigEndian.PutUint64(j.LeaseVS[:8], uint64(now))

			tr.Set(kv.Key, encodeJob(j))
			job = j
			return nil, nil
		}

		return nil, ErrNoJobs
	})

	if err != nil {
		return nil, err
	}
	return job, nil
}

// deleteJob removes a completed job
func (a *Automation[Deps]) deleteJob(job *Job) error {
	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		// Verify we still own the job
		value := tr.Get(job.Key).MustGet()
		if value == nil {
			return nil, nil // already deleted
		}

		current, err := decodeJob(job.Key, value)
		if err != nil {
			return nil, err
		}

		if current.OwnerID != a.workerID {
			return nil, ErrLeaseStolen
		}

		tr.Clear(job.Key)
		return nil, nil
	})
	return err
}

// retryJob increments attempts and sets backoff
func (a *Automation[Deps]) retryJob(job *Job, processErr error) error {
	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		// Verify we still own the job
		value := tr.Get(job.Key).MustGet()
		if value == nil {
			return nil, nil // already deleted
		}

		current, err := decodeJob(job.Key, value)
		if err != nil {
			return nil, err
		}

		if current.OwnerID != a.workerID {
			return nil, ErrLeaseStolen
		}

		current.Attempts++
		if int(current.Attempts) >= a.config.MaxAttempts {
			// Move to DLQ
			return nil, a.moveToDLQInTx(tr, job, processErr)
		}

		// Exponential backoff: 1min, 5min, 25min
		backoff := a.calculateBackoff(int(current.Attempts))
		current.VestingNs = time.Now().Add(backoff).UnixNano()
		current.OwnerID = [16]byte{} // release ownership
		current.ExpiryNs = 0

		tr.Set(job.Key, encodeJob(current))
		return nil, nil
	})
	return err
}

func (a *Automation[Deps]) calculateBackoff(attempt int) time.Duration {
	// Exponential: base * 5^(attempt-1)
	base := a.config.RetryBaseWait
	multiplier := 1
	for i := 1; i < attempt; i++ {
		multiplier *= 5
	}
	return base * time.Duration(multiplier)
}
