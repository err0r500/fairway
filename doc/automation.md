# FoundationDB Processing Queue

Transactional **exclusive** job queue with robust leasing, clock skew tolerance, high throughput.

## Key Layout (Tuple Layer)

```
queue = fdb.Directory.create("queues").open(db, queueName)
dlq   = fdb.Directory.create("dlq").open(db, queueName)

Job key:   queue.pack((index, random_20bytes))
Job value: [vesting_ns(8) | expiry_ns(8) | created_vs(14) | owner_id(16) | attempts(1) | payload]

DLQ key:   dlq.pack((timestamp, original_key))
DLQ value: [attempts(1) | last_error | payload]
```

## Delivery Semantics

| Operation | Guarantee | Pattern |
|-----------|-----------|---------|
| **Enqueue** | Unique position | Snapshot read last_index → write (index+1, rand) |
| **Dequeue** | **Exactly-one consumer** | Scan READY jobs → atomic lease acquisition |
| **Process** | At-least-once | Worker processes → verify ownership → delete |
| **Failover** | Automatic | Lease expiry + grace → available for other workers |

**Note**: Index may have gaps (concurrent enqueues). Jobs processed in approximate FIFO order.

## Transaction Boundaries

```
Dequeue(tr1) ─commit─> process(payload) ─> Process(tr2) ─commit─> done
                       [SIDE EFFECTS]
```

- **Dequeue**: range read (wide conflict surface)
- **process()**: outside transaction, side effects happen here
- **Process**: point read (narrow conflict surface)

Implication: side effects may execute even if Process fails/conflicts.
Mitigation: idempotency keys, ExtendLease() heartbeat.

## Job States (FSM)

```
PENDING ──(vesting ≤ now)──> READY ──(take_lease)──> LEASED ──(success)──> DELETED
                                                       │
                                               (failure)│
                                                       ▼
                                     (attempts < max)──> PENDING (backoff)
                                     (attempts ≥ max)──> DLQ
```

## Algorithms

### Enqueue

```go
func Enqueue(payload []byte) error {
    return db.Transact(func(tr fdb.Transaction) error {
        // Snapshot read: no conflict, allows concurrent enqueues
        lastKey := tr.Snapshot().GetRange(queue.Range(), fdb.RangeOptions{Limit: 1, Reverse: true})
        index := extractIndex(lastKey) + 1

        key := queue.Pack(tuple.Tuple{index, randomBytes(20)})
        value := encodeJob(vestingNs, 0, 0, nil, 0, payload) // VS placeholder filled by FDB
        tr.SetVersionstampedValue(key, value)
        return nil
    })
}
```

### Dequeue

```go
func Dequeue(workerID [16]byte) (*Job, error) {
    return db.Transact(func(tr fdb.Transaction) (*Job, error) {
        now := time.Now().UnixNano()
        readVersion := tr.GetReadVersion().MustGet()

        jobs := tr.GetRange(queue.Range(), fdb.RangeOptions{Limit: BatchSize})
        for _, kv := range jobs.GetSliceOrPanic() {
            job := decodeJob(kv)

            if job.VestingNs > now {
                continue // Not ready yet
            }

            if job.OwnerID == zeroID { // Unclaimed
                return claimJob(tr, job, workerID, now)
            }

            // Check if lease expired (hybrid clock)
            ntpExpired := now > job.ExpiryNs + GracePeriod
            vsExpired := readVersion - job.CreatedVersion > TTLVersions

            if ntpExpired || vsExpired {
                return claimJob(tr, job, workerID, now)
            }
        }
        return nil, ErrNoJobsAvailable
    })
}

func claimJob(tr fdb.Transaction, job *Job, workerID [16]byte, now int64) (*Job, error) {
    job.OwnerID = workerID
    job.ExpiryNs = now + LeaseTTL
    tr.SetVersionstampedValue(job.Key, encodeJob(job)) // Updates created_vs
    return job, nil
}
```

### Process with Ownership Verification

```go
func Process(job *Job, workerID [16]byte, process func([]byte) error) error {
    err := process(job.Payload)

    return db.Transact(func(tr fdb.Transaction) error {
        // Re-read and verify ownership before committing
        current := tr.Get(job.Key).MustGet()
        if current == nil {
            return ErrJobGone // Already processed
        }
        currentJob := decodeJob(current)
        if currentJob.OwnerID != workerID {
            return ErrLeaseStolen // Abort, let new owner handle
        }

        if err != nil {
            return handleFailure(tr, job, err)
        }

        tr.Clear(job.Key)
        return nil
    })
}
```

### Failure Handling

```go
const MaxAttempts = 3

func handleFailure(tr fdb.Transaction, job *Job, err error) error {
    job.Attempts++

    if job.Attempts >= MaxAttempts {
        // Move to DLQ
        dlqKey := dlq.Pack(tuple.Tuple{time.Now().UnixNano(), job.Key})
        dlqValue := encodeDLQ(job.Attempts, err.Error(), job.Payload)
        tr.Set(dlqKey, dlqValue)
        tr.Clear(job.Key)
        return nil
    }

    // Exponential backoff: 1min, 5min, 25min
    delay := time.Minute * time.Duration(math.Pow(5, float64(job.Attempts-1)))
    job.VestingNs = time.Now().Add(delay).UnixNano()
    job.OwnerID = zeroID // Release lease
    job.ExpiryNs = 0
    tr.Set(job.Key, encodeJob(job))
    return nil
}
```

### Lease Extension

```go
func ExtendLease(job *Job, workerID [16]byte) error {
    return db.Transact(func(tr fdb.Transaction) error {
        current := tr.Get(job.Key).MustGet()
        if current == nil || decodeJob(current).OwnerID != workerID {
            return ErrLeaseStolen
        }
        job.ExpiryNs = time.Now().UnixNano() + LeaseTTL
        tr.SetVersionstampedValue(job.Key, encodeJob(job))
        return nil
    })
}
```

## Clock Skew Handling (Hybrid QuiCK)

```
1. Primary:   NTP timestamps + GRACE_PERIOD (handles ±30s skew)
2. Fallback:  FDB versionstamps (monotonic, never lies)
3. Steal if:  NTP_EXPIRED || VERSION_EXPIRED

canSteal := (now > expiry_ns + GRACE_PERIOD) || (read_version - created_version > TTL_VERSIONS)
```

Versionstamp is the **primary safety mechanism**. NTP grace is belt-and-suspenders.

## Configuration

```go
const (
    LeaseTTL     = 30 * time.Second  // Max process time + margin
    GracePeriod  = 60 * time.Second  // Handles ±30s NTP skew
    BatchSize    = 16                // Throughput vs contention tradeoff
    TTLVersions  = 30_000            // ~30s @ 1k tx/s
    MaxAttempts  = 3                 // Then move to DLQ
)
// Note: FDB has 100KB value limit. For large payloads, store reference instead.
```

## Performance

```
O(1) enqueue      - snapshot read + single write
O(batch) dequeue  - sequential range scan
Zero hot-spots    - random key suffix
Horizontal scale  - N parallel queues
Throughput        - 10k-100k jobs/s (hardware dependent)
```

## Files

| File | Contents |
|------|----------|
| `queue.go` | Queue, Enqueue, Dequeue |
| `worker.go` | Process, ExtendLease, failure handling |
| `dlq.go` | DLQ reader, replay |
| `queue_test.go` | Tests |

## References

- [QuiCK: FDB Queue Paper](https://www.foundationdb.org/files/QuiCK.pdf) (Apple CloudKit, billions jobs/day)
