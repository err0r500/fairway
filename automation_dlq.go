package fairway

import (
	"encoding/binary"
	"errors"
	"iter"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// DLQEntry represents a failed job in the dead letter queue
type DLQEntry struct {
	Key        fdb.Key
	EnqueuedAt time.Time
	EventVS    dcb.Versionstamp
	Attempts   uint8
	Error      string
}

// DLQ value format:
// [event_vs:12][attempts:1][error_len:2][error:variable]
const dlqHeaderSize = 12 + 1 + 2 // 15 bytes

func encodeDLQ(job *Job, err error) []byte {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	if len(errStr) > 65535 {
		errStr = errStr[:65535]
	}

	buf := make([]byte, dlqHeaderSize+len(errStr))
	copy(buf[0:12], job.EventVS[:])
	buf[12] = job.Attempts
	binary.BigEndian.PutUint16(buf[13:15], uint16(len(errStr)))
	copy(buf[15:], errStr)
	return buf
}

func decodeDLQ(key fdb.Key, value []byte, dlqDir subspace.Subspace) (*DLQEntry, error) {
	if len(value) < dlqHeaderSize {
		return nil, errors.New("invalid DLQ value size")
	}

	errLen := binary.BigEndian.Uint16(value[13:15])
	if len(value) < dlqHeaderSize+int(errLen) {
		return nil, errors.New("invalid DLQ value: error truncated")
	}

	entry := &DLQEntry{
		Key:      key,
		Attempts: value[12],
		Error:    string(value[15 : 15+errLen]),
	}
	copy(entry.EventVS[:], value[0:12])

	// Extract timestamp from key: dlq/<ts>/<event_vs>
	keyTuple, err := dlqDir.Unpack(key)
	if err != nil {
		return nil, err
	}
	if len(keyTuple) >= 1 {
		if ts, ok := keyTuple[0].(int64); ok {
			entry.EnqueuedAt = time.Unix(0, ts)
		}
	}

	return entry, nil
}

// moveToDLQInTx moves a job to the DLQ within an existing transaction
func (a *Automation[Deps]) moveToDLQInTx(tr fdb.Transaction, job *Job, err error) error {
	// DLQ key: dlq/<timestamp>/<event_vs>
	ts := time.Now().UnixNano()

	var txVersion [10]byte
	copy(txVersion[:], job.EventVS[:10])
	userVersion := binary.BigEndian.Uint16(job.EventVS[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	dlqKey := a.dlqDir.Pack(tuple.Tuple{ts, tupleVs})
	tr.Set(dlqKey, encodeDLQ(job, err))
	tr.Clear(job.Key)
	return nil
}

// ListDLQ returns an iterator over all DLQ entries
func (a *Automation[Deps]) ListDLQ() iter.Seq2[DLQEntry, error] {
	return func(yield func(DLQEntry, error) bool) {
		_, err := a.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
			iter := tr.GetRange(a.dlqDir, fdb.RangeOptions{}).Iterator()
			for iter.Advance() {
				kv, err := iter.Get()
				if err != nil {
					if !yield(DLQEntry{}, err) {
						return nil, nil
					}
					continue
				}

				entry, err := decodeDLQ(kv.Key, kv.Value, a.dlqDir)
				if err != nil {
					if !yield(DLQEntry{}, err) {
						return nil, nil
					}
					continue
				}

				if !yield(*entry, nil) {
					return nil, nil
				}
			}
			return nil, nil
		})
		if err != nil {
			yield(DLQEntry{}, err)
		}
	}
}

// ReplayDLQ moves a DLQ entry back to the queue for reprocessing
func (a *Automation[Deps]) ReplayDLQ(dlqKey fdb.Key) error {
	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		value := tr.Get(dlqKey).MustGet()
		if value == nil {
			return nil, errors.New("DLQ entry not found")
		}

		entry, err := decodeDLQ(dlqKey, value, a.dlqDir)
		if err != nil {
			return nil, err
		}

		// Re-enqueue the event
		if err := a.enqueueInTx(tr, entry.EventVS); err != nil {
			return nil, err
		}

		// Remove from DLQ
		tr.Clear(dlqKey)
		return nil, nil
	})
	return err
}

// PurgeDLQ removes all DLQ entries older than the given time
func (a *Automation[Deps]) PurgeDLQ(before time.Time) error {
	_, err := a.db.Transact(func(tr fdb.Transaction) (any, error) {
		ts := before.UnixNano()

		// Build range: dlq/<0> to dlq/<before_ts>
		beginKey := a.dlqDir.Pack(tuple.Tuple{int64(0)})
		endKey := a.dlqDir.Pack(tuple.Tuple{ts})

		tr.ClearRange(fdb.KeyRange{Begin: beginKey, End: endKey})
		return nil, nil
	})
	return err
}
