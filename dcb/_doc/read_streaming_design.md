# K-Way Merge Streaming Reads

## Purpose

Query execution in `dcb/read.go` is designed to stream results end-to-end without
collecting all versionstamps into memory first. This is achieved with a k-way
merge over multiple FoundationDB range iterators, producing a single ordered
stream of events.

## Why A K-Way Merge

Each query item can expand into multiple FoundationDB ranges (e.g., per type,
per tag combination). Each range is already ordered by versionstamp because the
versionstamp is at the end of the index key. To return a single globally ordered
result set across all ranges, the implementation merges these ordered streams in
real time.

## Core Components

- `rangeIterator`: Wraps an `fdb.RangeIterator` and keeps its current key and
  current versionstamp.
- `vsHeap`: A min-heap ordered by versionstamp (stable for equal versionstamps).
- `readEvents`: Builds iterators for all ranges, merges them, and yields events
  as they are discovered.

## Algorithm Overview

1. Build query ranges
   - For each query item, `buildQueryRanges` returns one or more ranges.
   - Each range corresponds to a stream of versionstamps.

2. Initialize one iterator per range
   - `initRangeIterator` advances each iterator to its first item and extracts
     its versionstamp.

3. Merge all iterators with a min-heap
   - Each iterator is pushed into `vsHeap` using its current versionstamp.
   - The smallest versionstamp is popped, emitted, and then advanced.

4. Deduplicate across ranges
   - If multiple ranges surface the same versionstamp, only the first one is
     emitted. The rest are advanced and re-queued.

5. Stream results
   - Each emitted versionstamp is used to fetch the full event from primary
     storage and yielded immediately to the caller.

## Pseudocode

```text
for each range in buildQueryRanges(query):
    iterator = initRangeIterator(range)
    if not exhausted: push iterator into min-heap

lastEmitted = nil
while heap not empty:
    iterator = heap.pop_min()
    vs = iterator.current_versionstamp

    if lastEmitted == vs:
        iterator.advance()
        if not exhausted: heap.push(iterator)
        continue

    event = fetchEvent(vs)
    yield event
    lastEmitted = vs

    iterator.advance()
    if not exhausted: heap.push(iterator)
```

## Streaming Behavior

The merge loop never buffers all results. It only keeps one current item per
range plus the heap, so memory usage is proportional to the number of ranges,
not the number of events. This enables full streaming of large result sets.

## Ordering Guarantees

- Global ordering is by versionstamp.
- Ties are broken stably by the original iterator index to keep the merge
  deterministic even when multiple ranges contain the same versionstamp.

## Limits And Early Exit

`readEvents` stops emitting if:
- the caller stops the iterator (yield returns false), or
- the `Limit` option is reached.

The merge terminates cleanly without scanning additional ranges.

## Where To Look

The implementation lives in `dcb/read.go`:
- `readEvents`: main streaming merge loop
- `rangeIterator`, `initRangeIterator`, `advance`: range iterator state
- `vsHeap`: min-heap used for the k-way merge
