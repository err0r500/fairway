# Streaming Reads

The `dcb/read.go` implementation streams events end-to-end without buffering all results in memory. It uses a **k-way merge** over multiple FoundationDB range iterators to produce a single globally ordered event stream.

---

## Why K-Way Merge?

A single `Query` can expand into multiple FDB ranges — for example, one range per event type, or one range per tag combination. Each individual range is already ordered by versionstamp (since versionstamp is the last element in the key). The merge combines these sorted streams in real time to produce a single ordered result.

---

## Components

### `rangeIterator`

Wraps an `fdb.RangeIterator`. Keeps track of:

- The current FDB key/value
- The current extracted versionstamp

### `vsHeap`

A min-heap ordered by versionstamp. When two iterators have equal versionstamps, ordering is broken by the original iterator index for determinism.

### `readEvents`

The main driver: builds all ranges, initializes iterators, runs the merge loop, and yields events one at a time via `iter.Seq2`.

---

## Algorithm

```text
for each range in buildQueryRanges(query):
    iterator = initRangeIterator(range)
    if not exhausted: push iterator into min-heap

lastEmitted = nil

while heap not empty:
    iterator = heap.pop_min()           // smallest versionstamp
    vs = iterator.current_versionstamp

    if vs == lastEmitted:               // deduplication
        iterator.advance()
        if not exhausted: heap.push(iterator)
        continue

    event = fetchEvent(primaryStore, vs) // lookup from /e/<vs>
    yield event
    lastEmitted = vs

    iterator.advance()
    if not exhausted: heap.push(iterator)
```

---

## Memory Profile

The merge loop never buffers all results. It holds:

- One current item per active range iterator
- The heap (one entry per active iterator)

Memory usage is **O(number of ranges)**, not O(number of events). A query with 3 types across 2 tag combinations → at most 6 iterators in memory simultaneously, regardless of result set size.

---

## Ordering Guarantees

- Events are always yielded in ascending versionstamp order.
- Equal versionstamps (which cannot occur for distinct events but may appear across ranges pointing to the same event) are deduplicated — each event is emitted exactly once.
- Deduplication is stable: if multiple ranges point to the same versionstamp, the first one wins.

---

## Early Exit

The merge terminates cleanly when:

- The caller stops the iterator (returns `false` from its range handler)
- The configured `Limit` in `ReadOptions` is reached

No additional FDB ranges are scanned after the stop condition is met.

---

## Go Iterator Integration

Results are yielded via Go 1.23's `iter.Seq2[StoredEvent, error]`:

```go
for event, err := range store.Read(ctx, query, nil) {
    if err != nil {
        return err
    }
    // process event
}
```

The caller controls iteration pace. The store does not pre-fetch or buffer beyond what is needed for the current merge step.
