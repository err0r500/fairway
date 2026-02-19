package fairway

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway/dcb"
)

// ScopedTx wraps fdb.Transaction, auto-prefixing keys to ReadModel's subspace
type ScopedTx struct {
	tr    fdb.Transaction
	space subspace.Subspace
}

func (s ScopedTx) Set(key tuple.Tuple, value []byte) {
	s.tr.Set(s.space.Pack(key), value)
}

func (s ScopedTx) Get(key tuple.Tuple) fdb.FutureByteSlice {
	return s.tr.Get(s.space.Pack(key))
}

func (s ScopedTx) Clear(key tuple.Tuple) {
	s.tr.Clear(s.space.Pack(key))
}

func (s ScopedTx) ClearRange(begin, end tuple.Tuple) {
	s.tr.ClearRange(fdb.KeyRange{
		Begin: s.space.Pack(begin),
		End:   s.space.Pack(end),
	})
}

func (s ScopedTx) GetRange(prefix tuple.Tuple, opts fdb.RangeOptions) fdb.RangeResult {
	return s.tr.GetRange(s.space.Sub(prefix...), opts)
}

// ReadModelConfig configures read model behavior
type ReadModelConfig struct {
	BatchSize    int
	PollInterval time.Duration
}

func defaultReadModelConfig() ReadModelConfig {
	return ReadModelConfig{
		BatchSize:    100,
		PollInterval: 100 * time.Millisecond,
	}
}

// ReadModel processes events in order to maintain a persistent projection.
// It stores a cursor in FDB so it can resume from where it left off on restart.
// T is the type of values stored in the read model's data space.
type ReadModel[T any] struct {
	name          string
	eventTypes    []string
	eventRegistry eventRegistry
	handler       func(ScopedTx, Event) error
	config        ReadModelConfig

	db          fdb.Database
	typeIndexes []subspace.Subspace // namespace/t/<type> per event type
	eventsSpace subspace.Subspace   // namespace/e
	cursorKey   fdb.Key             // namespace/rm/<name>/cursor
	dataSpace   subspace.Subspace   // namespace/rm/<name>/data

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	errCh      chan error
	pollTicker *time.Ticker
}

// ReadModelOption configures a ReadModel
type ReadModelOption[T any] func(*ReadModel[T])

// WithReadModelBatchSize sets the batch size for event processing
func WithReadModelBatchSize[T any](n int) ReadModelOption[T] {
	return func(rm *ReadModel[T]) {
		if n > 0 {
			rm.config.BatchSize = n
		}
	}
}

// WithReadModelPollInterval sets the polling interval
func WithReadModelPollInterval[T any](d time.Duration) ReadModelOption[T] {
	return func(rm *ReadModel[T]) {
		if d > 0 {
			rm.config.PollInterval = d
		}
	}
}

// NewReadModel creates a new persistent read model.
// name uniquely identifies this projection (used for cursor storage).
// eventTypeExamples are zero-value instances of each event type to watch.
// handler is called for each event in versionstamp order.
func NewReadModel[T any](
	store dcb.DcbStore,
	name string,
	eventTypeExamples []any,
	handler func(ScopedTx, Event) error,
	opts ...ReadModelOption[T],
) (*ReadModel[T], error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if name == "" {
		return nil, errors.New("name is required")
	}
	if handler == nil {
		return nil, errors.New("handler is required")
	}
	if len(eventTypeExamples) == 0 {
		return nil, errors.New("at least one event type example is required")
	}

	ns := store.Namespace()
	dcbRoot := subspace.Sub(ns)

	registry := newEventRegistry()
	eventTypes := make([]string, 0, len(eventTypeExamples))
	typeIndexes := make([]subspace.Subspace, 0, len(eventTypeExamples))

	for _, ex := range eventTypeExamples {
		typeName := resolveEventTypeName(ex)
		eventTypes = append(eventTypes, typeName)
		registry.types[typeName] = reflect.TypeOf(ex)
		typeIndexes = append(typeIndexes, dcbRoot.Sub("t").Sub(typeName))
	}

	rmRoot := dcbRoot.Sub("rm").Sub(name)
	cursorKey := rmRoot.Pack(tuple.Tuple{"cursor"})
	dataSpace := rmRoot.Sub("data")

	rm := &ReadModel[T]{
		name:          name,
		eventTypes:    eventTypes,
		eventRegistry: registry,
		handler:       handler,
		config:        defaultReadModelConfig(),
		db:            store.Database(),
		typeIndexes:   typeIndexes,
		eventsSpace:   dcbRoot.Sub("e"),
		cursorKey:     cursorKey,
		dataSpace:     dataSpace,
		errCh:         make(chan error, 100),
	}

	for _, opt := range opts {
		opt(rm)
	}

	return rm, nil
}

// Start begins read model processing
func (rm *ReadModel[T]) Start(ctx context.Context) error {
	rm.ctx, rm.cancel = context.WithCancel(ctx)
	rm.pollTicker = time.NewTicker(rm.config.PollInterval)

	rm.wg.Add(1)
	go rm.runWatch()

	return nil
}

// Stop gracefully stops the read model
func (rm *ReadModel[T]) Stop() {
	if rm.cancel != nil {
		rm.cancel()
	}
	if rm.pollTicker != nil {
		rm.pollTicker.Stop()
	}
}

// Wait blocks until all goroutines finish and returns any accumulated errors
func (rm *ReadModel[T]) Wait() error {
	rm.wg.Wait()
	close(rm.errCh)

	var errs []error
	for err := range rm.errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Errors returns the error channel for monitoring
func (rm *ReadModel[T]) Errors() <-chan error {
	return rm.errCh
}

// runWatch is the Watch polling loop
func (rm *ReadModel[T]) runWatch() {
	defer rm.wg.Done()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-rm.pollTicker.C:
			if err := rm.processNextBatch(); err != nil {
				select {
				case rm.errCh <- fmt.Errorf("read model %q: %w", rm.name, err):
				default:
				}
			}
		}
	}
}

// vsRawEvent pairs a versionstamp with raw event data for deferred decoding
type vsRawEvent struct {
	vs    dcb.Versionstamp
	event dcb.Event
}

// processNextBatch fetches and processes the next batch of events, then updates the cursor
func (rm *ReadModel[T]) processNextBatch() error {
	batch, err := rm.fetchBatch()
	if err != nil {
		return fmt.Errorf("fetch batch: %w", err)
	}
	if len(batch) == 0 {
		return nil
	}

	_, err = rm.db.Transact(func(tr fdb.Transaction) (any, error) {
		stx := ScopedTx{tr: tr, space: rm.dataSpace}
		var lastVS dcb.Versionstamp
		for _, item := range batch {
			ev, err := rm.eventRegistry.deserialize(item.event)
			if err != nil {
				return nil, fmt.Errorf("deserialize event at %x: %w", item.vs[:], err)
			}
			if err := rm.handler(stx, ev); err != nil {
				return nil, fmt.Errorf("handler at %x: %w", item.vs[:], err)
			}
			lastVS = item.vs
		}
		tr.Set(rm.cursorKey, lastVS[:])
		return nil, nil
	})
	return err
}

// fetchBatch reads up to BatchSize events after the current cursor from all watched type indexes.
// Events are returned in versionstamp order (global event order).
func (rm *ReadModel[T]) fetchBatch() ([]vsRawEvent, error) {
	var batch []vsRawEvent

	_, err := rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		batch = nil // reset on FDB retry

		// Read cursor
		cursorValue := tr.Get(rm.cursorKey).MustGet()
		var cursor *dcb.Versionstamp
		if len(cursorValue) == 12 {
			var vs dcb.Versionstamp
			copy(vs[:], cursorValue)
			cursor = &vs
		}

		// Collect versionstamps from each type index
		var allVS []dcb.Versionstamp
		seen := make(map[dcb.Versionstamp]bool)

		for _, typeIndex := range rm.typeIndexes {
			var r fdb.Range
			if cursor != nil {
				rng, err := rangeAfterVersionstamp(typeIndex, *cursor)
				if err != nil {
					return nil, err
				}
				r = rng
			} else {
				r = typeIndex
			}

			kvs := tr.GetRange(r, fdb.RangeOptions{Limit: rm.config.BatchSize}).GetSliceOrPanic()
			for _, kv := range kvs {
				vs := extractVersionstampFromTypeIndex(typeIndex, kv.Key)
				if vs == (dcb.Versionstamp{}) || seen[vs] {
					continue
				}
				seen[vs] = true
				allVS = append(allVS, vs)
			}
		}

		if len(allVS) == 0 {
			return nil, nil
		}

		// Sort by versionstamp to ensure global event order
		sort.Slice(allVS, func(i, j int) bool {
			return allVS[i].Compare(allVS[j]) < 0
		})

		// Limit to BatchSize
		if len(allVS) > rm.config.BatchSize {
			allVS = allVS[:rm.config.BatchSize]
		}

		// Fetch raw event data for each versionstamp
		batch = make([]vsRawEvent, 0, len(allVS))
		for _, vs := range allVS {
			event, err := rm.fetchRawEvent(tr, vs)
			if err != nil {
				return nil, err
			}
			batch = append(batch, vsRawEvent{vs: vs, event: event})
		}

		return nil, nil
	})

	return batch, err
}

// fetchRawEvent reads and decodes a single event from the events subspace
func (rm *ReadModel[T]) fetchRawEvent(tr fdb.ReadTransaction, vs dcb.Versionstamp) (dcb.Event, error) {
	var txVersion [10]byte
	copy(txVersion[:], vs[:10])
	userVersion := binary.BigEndian.Uint16(vs[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	eventKey := rm.eventsSpace.Pack(tuple.Tuple{tupleVs})
	encodedValue := tr.Get(eventKey).MustGet()
	if encodedValue == nil {
		return dcb.Event{}, fmt.Errorf("event not found at versionstamp %x", vs[:])
	}

	eventTuple, err := tuple.Unpack(encodedValue)
	if err != nil {
		return dcb.Event{}, fmt.Errorf("unpack event at %x: %w", vs[:], err)
	}
	if len(eventTuple) != 3 {
		return dcb.Event{}, fmt.Errorf("expected 3-tuple at %x, got %d elements", vs[:], len(eventTuple))
	}

	eventType, ok := eventTuple[0].(string)
	if !ok {
		return dcb.Event{}, fmt.Errorf("type field at %x is %T, expected string", vs[:], eventTuple[0])
	}

	var tags []string
	if eventTuple[1] != nil {
		tagsTuple, ok := eventTuple[1].(tuple.Tuple)
		if !ok {
			return dcb.Event{}, fmt.Errorf("tags field at %x is %T, expected tuple", vs[:], eventTuple[1])
		}
		tags = make([]string, len(tagsTuple))
		for i, t := range tagsTuple {
			tags[i] = t.(string)
		}
	}

	eventData, ok := eventTuple[2].([]byte)
	if !ok {
		return dcb.Event{}, fmt.Errorf("data field at %x is %T, expected []byte", vs[:], eventTuple[2])
	}

	return dcb.Event{Type: eventType, Tags: tags, Data: eventData}, nil
}

// Get retrieves values from the read model's data space.
// Returns a slice of pointers; nil entries indicate missing keys.
func (rm *ReadModel[T]) Get(keys ...tuple.Tuple) ([]*T, error) {
	var results []*T
	_, err := rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		results = make([]*T, len(keys))
		for i, key := range keys {
			data := tr.Get(rm.dataSpace.Pack(key)).MustGet()
			if data == nil {
				continue
			}
			var v T
			if err := json.Unmarshal(data, &v); err != nil {
				return nil, fmt.Errorf("unmarshal key %v: %w", key, err)
			}
			results[i] = &v
		}
		return nil, nil
	})
	return results, err
}
