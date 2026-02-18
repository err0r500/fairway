package fairway

import (
	"context"
	"encoding/binary"
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
type ReadModel struct {
	name          string
	eventTypes    []string
	eventRegistry eventRegistry
	handler       func(Event) error
	config        ReadModelConfig

	db          fdb.Database
	typeIndexes []subspace.Subspace // namespace/t/<type> per event type
	eventsSpace subspace.Subspace   // namespace/e
	cursorKey   fdb.Key             // namespace/rm/<name>/cursor

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	errCh      chan error
	pollTicker *time.Ticker
}

// ReadModelOption configures a ReadModel
type ReadModelOption func(*ReadModel)

// WithReadModelBatchSize sets the batch size for event processing
func WithReadModelBatchSize(n int) ReadModelOption {
	return func(rm *ReadModel) {
		if n > 0 {
			rm.config.BatchSize = n
		}
	}
}

// WithReadModelPollInterval sets the polling interval
func WithReadModelPollInterval(d time.Duration) ReadModelOption {
	return func(rm *ReadModel) {
		if d > 0 {
			rm.config.PollInterval = d
		}
	}
}

// NewReadModel creates a new persistent read model.
// name uniquely identifies this projection (used for cursor storage).
// eventTypeExamples are zero-value instances of each event type to watch.
// handler is called for each event in versionstamp order.
func NewReadModel(
	store dcb.DcbStore,
	name string,
	eventTypeExamples []any,
	handler func(Event) error,
	opts ...ReadModelOption,
) (*ReadModel, error) {
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

	cursorKey := dcbRoot.Sub("rm").Sub(name).Pack(tuple.Tuple{"cursor"})

	rm := &ReadModel{
		name:          name,
		eventTypes:    eventTypes,
		eventRegistry: registry,
		handler:       handler,
		config:        defaultReadModelConfig(),
		db:            store.Database(),
		typeIndexes:   typeIndexes,
		eventsSpace:   dcbRoot.Sub("e"),
		cursorKey:     cursorKey,
		errCh:         make(chan error, 100),
	}

	for _, opt := range opts {
		opt(rm)
	}

	return rm, nil
}

// Start begins read model processing
func (rm *ReadModel) Start(ctx context.Context) error {
	rm.ctx, rm.cancel = context.WithCancel(ctx)
	rm.pollTicker = time.NewTicker(rm.config.PollInterval)

	rm.wg.Add(1)
	go rm.runWatch()

	return nil
}

// Stop gracefully stops the read model
func (rm *ReadModel) Stop() {
	if rm.cancel != nil {
		rm.cancel()
	}
	if rm.pollTicker != nil {
		rm.pollTicker.Stop()
	}
}

// Wait blocks until all goroutines finish and returns any accumulated errors
func (rm *ReadModel) Wait() error {
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
func (rm *ReadModel) Errors() <-chan error {
	return rm.errCh
}

// runWatch is the Watch polling loop
func (rm *ReadModel) runWatch() {
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
func (rm *ReadModel) processNextBatch() error {
	// Phase 1: collect events in a read transaction (no side effects)
	batch, err := rm.fetchBatch()
	if err != nil {
		return fmt.Errorf("fetch batch: %w", err)
	}

	if len(batch) == 0 {
		return nil
	}

	// Phase 2: process events (outside any transaction)
	var lastVS dcb.Versionstamp
	for _, item := range batch {
		ev, err := rm.eventRegistry.deserialize(item.event)
		if err != nil {
			return fmt.Errorf("deserialize event at %x: %w", item.vs[:], err)
		}

		if err := rm.handler(ev); err != nil {
			return fmt.Errorf("handler at %x: %w", item.vs[:], err)
		}

		lastVS = item.vs
	}

	// Phase 3: persist cursor (only reached if all handlers succeeded)
	if _, err := rm.db.Transact(func(tr fdb.Transaction) (any, error) {
		tr.Set(rm.cursorKey, lastVS[:])
		return nil, nil
	}); err != nil {
		return fmt.Errorf("update cursor: %w", err)
	}

	return nil
}

// fetchBatch reads up to BatchSize events after the current cursor from all watched type indexes.
// Events are returned in versionstamp order (global event order).
func (rm *ReadModel) fetchBatch() ([]vsRawEvent, error) {
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
func (rm *ReadModel) fetchRawEvent(tr fdb.ReadTransaction, vs dcb.Versionstamp) (dcb.Event, error) {
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
