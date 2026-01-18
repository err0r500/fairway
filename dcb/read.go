package dcb

import (
	"container/heap"
	"context"
	"encoding/binary"
	"errors"
	"iter"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// Read returns events matching the query as an iterator sequence.
func (s fdbStore) Read(ctx context.Context, query Query, opts *ReadOptions) iter.Seq2[StoredEvent, error] {
	return func(yield func(StoredEvent, error) bool) {
		// Check context before starting work
		if err := ctx.Err(); err != nil {
			yield(StoredEvent{}, err)
			return
		}

		if opts == nil {
			opts = &ReadOptions{}
		}

		start := time.Now()
		eventCount := 0

		// Execute read in transaction
		_, err := s.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
			count, err := s.readEvents(ctx, tr, query, opts.After, opts, yield)
			eventCount = count
			return nil, err
		})

		duration := time.Since(start)
		success := err == nil

		s.metrics.RecordReadDuration(duration, success)
		if success {
			s.metrics.RecordReadEvents(eventCount)
			s.logger.Info("read completed", "event_count", eventCount, "duration", duration)
		} else {
			s.logger.Error("read failed", err, "duration", duration)
			yield(StoredEvent{}, err)
		}
	}
}

// initRangeIterator creates an iterator for a range and advances it to the first valid item.
// Returns nil if the iterator is empty.
func initRangeIterator(tr fdb.ReadTransaction, r fdb.Range) (*rangeIterator, error) {
	iter := tr.GetRange(r, fdb.RangeOptions{}).Iterator()
	ri := &rangeIterator{
		iter:      iter,
		exhausted: false,
	}

	// Advance to first valid item
	if iter.Advance() {
		kv, err := iter.Get()
		if err != nil {
			return nil, err
		}

		vs := extractVersionstamp(kv.Key)
		ri.currentKey = kv.Key
		ri.currentVS = vs
		return ri, nil
	}

	// Iterator empty
	ri.exhausted = true
	return ri, nil
}

// advance moves the iterator to the next valid item.
// Returns true if advanced successfully, false if exhausted.
func (ri *rangeIterator) advance() (bool, error) {
	if ri.iter.Advance() {
		kv, err := ri.iter.Get()
		if err != nil {
			return false, err
		}

		vs := extractVersionstamp(kv.Key)
		ri.currentKey = kv.Key
		ri.currentVS = vs
		return true, nil
	}

	ri.exhausted = true
	return false, nil
}

// fetchEvent retrieves the full event data for a given versionstamp.
func (s fdbStore) fetchEvent(ctx context.Context, tr fdb.ReadTransaction, vs Versionstamp) (StoredEvent, error) {
	// Convert 12-byte versionstamp to tuple.Versionstamp for packing
	var txVersion [10]byte
	copy(txVersion[:], vs[:10])
	userVersion := binary.BigEndian.Uint16(vs[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	eventKey := s.events.Pack(tuple.Tuple{tupleVs})
	encodedValue := tr.Get(eventKey).MustGet()

	if encodedValue == nil {
		// Event not found (shouldn't happen)
		if ctx.Err() != nil {
			return StoredEvent{}, ctx.Err()
		}
		return StoredEvent{}, errors.New("event data not found")
	}

	event, err := decodeEvent(ctx, encodedValue)
	if err != nil {
		return StoredEvent{}, err
	}

	return StoredEvent{Type: event.Type, Data: event.Data, Position: vs}, nil
}

// readEvents reads events from the transaction using k-way merge for streaming.
// All queries use k-way merge - fully streaming, no collect-and-sort.
func (s fdbStore) readEvents(
	ctx context.Context,
	tr fdb.ReadTransaction,
	query Query,
	after *Versionstamp,
	opts *ReadOptions,
	yield func(StoredEvent, error) bool) (int, error) {
	// Build all ranges (buildQueryRanges now handles type discovery for tags-only)
	var allIterators []*rangeIterator

	for _, item := range query.Items {
		ranges, err := s.buildQueryRanges(tr, item, after)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			return 0, err
		}

		// Create streaming iterator for each range
		for _, r := range ranges {
			ri, err := initRangeIterator(tr, r)
			if err != nil {
				if ctx.Err() != nil {
					return 0, ctx.Err()
				}
				return 0, err
			}
			if !ri.exhausted {
				allIterators = append(allIterators, ri)
			}
		}
	}

	// Build min-heap from all iterators
	h := &vsHeap{}
	heap.Init(h)
	for i, ri := range allIterators {
		heap.Push(h, heapItem{iter: ri, index: i})
	}

	// K-way merge with deduplication
	var lastEmitted *Versionstamp
	eventCount := 0

	for h.Len() > 0 {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return eventCount, ctx.Err()
		default:
		}

		// Pop iterator with smallest versionstamp
		item := heap.Pop(h).(heapItem)
		ri := item.iter
		currentVS := ri.currentVS

		// Deduplicate: skip if same as last emitted
		if lastEmitted != nil && currentVS.Compare(*lastEmitted) == 0 {
			// Advance and re-push if not exhausted
			advanced, err := ri.advance()
			if err != nil {
				if ctx.Err() != nil {
					return eventCount, ctx.Err()
				}
				return eventCount, err
			}
			if advanced {
				heap.Push(h, item)
			}
			continue
		}

		// Fetch and yield event
		storedEvent, err := s.fetchEvent(ctx, tr, currentVS)
		if err != nil {
			return eventCount, err
		}

		if !yield(storedEvent, nil) {
			return eventCount, nil
		}

		lastEmitted = &currentVS
		eventCount++

		// Check limit
		if opts.Limit > 0 && eventCount >= opts.Limit {
			return eventCount, nil
		}

		// Advance this iterator and re-push if not exhausted
		advanced, err := ri.advance()
		if err != nil {
			if ctx.Err() != nil {
				return eventCount, ctx.Err()
			}
			return eventCount, err
		}
		if advanced {
			heap.Push(h, item)
		}
	}

	return eventCount, nil
}

func decodeEvent(ctx context.Context, encodedValue []byte) (*Event, error) {
	// Decode event (type, data)
	// Tags are not stored, they are derived from event data
	eventTuple, err := tuple.Unpack(encodedValue)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}

	if len(eventTuple) != 2 {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("invalid event encoding")
	}

	// Extract type
	eventType, ok := eventTuple[0].(string)
	if !ok {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("invalid event type")
	}

	// Extract data
	eventData, ok := eventTuple[1].([]byte)
	if !ok {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("invalid event data")
	}

	return &Event{Type: eventType, Tags: nil, Data: eventData}, nil
}

// ReadAll returns all events in the store as an iterator sequence, ordered by versionstamp.
// Efficiently handles millions of events by streaming directly from the events subspace.
func (s fdbStore) ReadAll(ctx context.Context) iter.Seq2[StoredEvent, error] {
	return func(yield func(StoredEvent, error) bool) {
		if err := ctx.Err(); err != nil {
			yield(StoredEvent{}, err)
			return
		}

		start := time.Now()
		eventCount := 0

		_, err := s.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
			// Scan entire events subspace
			rangeOpts := fdb.RangeOptions{
				Limit: 1000, // Batch size hint for efficient streaming
			}

			iter := tr.GetRange(s.events, rangeOpts).Iterator()
			for iter.Advance() {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}

				kv, err := iter.Get()
				if err != nil {
					if ctx.Err() != nil {
						return nil, ctx.Err()
					}
					return nil, err
				}

				// Extract versionstamp from key
				keyTuple, err := s.events.Unpack(kv.Key)
				if err != nil {
					return nil, err
				}
				if len(keyTuple) != 1 {
					return nil, errors.New("invalid event key")
				}
				tupleVs, ok := keyTuple[0].(tuple.Versionstamp)
				if !ok {
					return nil, errors.New("invalid versionstamp in key")
				}

				// Convert to our Versionstamp type
				var vs Versionstamp
				copy(vs[:10], tupleVs.TransactionVersion[:])
				binary.BigEndian.PutUint16(vs[10:12], tupleVs.UserVersion)

				// Decode event
				event, err := decodeEvent(ctx, kv.Value)
				if err != nil {
					return nil, err
				}

				if !yield(StoredEvent{Type: event.Type, Data: event.Data, Position: vs}, nil) {
					return nil, nil
				}
				eventCount++
			}

			return nil, nil
		})

		duration := time.Since(start)
		success := err == nil

		s.metrics.RecordReadDuration(duration, success)
		if success {
			s.metrics.RecordReadEvents(eventCount)
			s.logger.Info("read all completed", "event_count", eventCount, "duration", duration)
		} else {
			s.logger.Error("read all failed", err, "duration", duration)
			yield(StoredEvent{}, err)
		}
	}
}

// rangeIterator wraps FDB iterator with current state for k-way merge
type rangeIterator struct {
	iter       *fdb.RangeIterator
	currentKey fdb.Key
	currentVS  Versionstamp
	exhausted  bool
}

// heapItem represents one iterator in the min-heap
type heapItem struct {
	iter  *rangeIterator
	index int // For stable sorting when versionstamps are equal
}

// vsHeap implements heap.Interface for min-heap ordered by versionstamp
type vsHeap []heapItem

func (h vsHeap) Len() int { return len(h) }

func (h vsHeap) Less(i, j int) bool {
	// Stable sort: if versionstamps equal, use original index
	cmp := h[i].iter.currentVS.Compare(h[j].iter.currentVS)
	return cmp <= 0
}

func (h vsHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *vsHeap) Push(x any) {
	*h = append(*h, x.(heapItem))
}

func (h *vsHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}
