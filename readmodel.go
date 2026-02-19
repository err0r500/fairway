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

// Path represents a hierarchical key path for the read model
type Path []string

// P creates a Path from variadic string segments
func P(segments ...string) Path { return segments }

func pathToTuple(p Path) tuple.Tuple {
	t := make(tuple.Tuple, len(p))
	for i, s := range p {
		t[i] = s
	}
	return t
}

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

// SetJSON marshals v to JSON and stores it at the given path
func (s ScopedTx) SetJSON(key Path, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.tr.Set(s.space.Pack(pathToTuple(key)), data)
	return nil
}

// ClearPath removes the value at the given path
func (s ScopedTx) ClearPath(key Path) {
	s.tr.Clear(s.space.Pack(pathToTuple(key)))
}

// SetPath sets an empty marker at the given path (for key-only storage)
func (s ScopedTx) SetPath(key Path) {
	s.tr.Set(s.space.Pack(pathToTuple(key)), nil)
}

// GetPath returns the value at the given path
func (s ScopedTx) GetPath(key Path) fdb.FutureByteSlice {
	return s.tr.Get(s.space.Pack(pathToTuple(key)))
}

// ScanPath returns all keys with the given prefix
func (s ScopedTx) ScanPath(prefix Path) []Path {
	kvs := s.tr.GetRange(s.space.Sub(pathToTuple(prefix)...), fdb.RangeOptions{}).GetSliceOrPanic()
	results := make([]Path, 0, len(kvs))
	for _, kv := range kvs {
		keyTuple, err := s.space.Unpack(kv.Key)
		if err != nil {
			continue
		}
		path := make(Path, len(keyTuple))
		for i, elem := range keyTuple {
			if str, ok := elem.(string); ok {
				path[i] = str
			}
		}
		results = append(results, path)
	}
	return results
}

// ClearPrefix removes all keys with the given prefix
func (s ScopedTx) ClearPrefix(prefix Path) {
	s.tr.ClearRange(s.space.Sub(pathToTuple(prefix)...))
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
// R is the repository type created by the RepoFactory for each transaction.
type ReadModel[T any, R any] struct {
	name          string
	eventTypes    []string
	eventRegistry eventRegistry
	repoFactory   func(fdb.Transaction, subspace.Subspace) R
	handler       func(R, Event) error
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

	caughtUp bool // true after processing a batch with no events
}

// ReadModelOption configures a ReadModel
type ReadModelOption[T any, R any] func(*ReadModel[T, R])

// WithReadModelBatchSize sets the batch size for event processing
func WithReadModelBatchSize[T any, R any](n int) ReadModelOption[T, R] {
	return func(rm *ReadModel[T, R]) {
		if n > 0 {
			rm.config.BatchSize = n
		}
	}
}

// WithReadModelPollInterval sets the polling interval
func WithReadModelPollInterval[T any, R any](d time.Duration) ReadModelOption[T, R] {
	return func(rm *ReadModel[T, R]) {
		if d > 0 {
			rm.config.PollInterval = d
		}
	}
}

// NewReadModel creates a new persistent read model.
// name uniquely identifies this projection (used for cursor storage).
// eventTypeExamples are zero-value instances of each event type to watch.
// repoFactory creates a repository for each transaction.
// handler is called for each event in versionstamp order.
func NewReadModel[T any, R any](
	store dcb.DcbStore,
	name string,
	eventTypeExamples []any,
	repoFactory func(fdb.Transaction, subspace.Subspace) R,
	handler func(R, Event) error,
	opts ...ReadModelOption[T, R],
) (*ReadModel[T, R], error) {
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

	rm := &ReadModel[T, R]{
		name:          name,
		eventTypes:    eventTypes,
		eventRegistry: registry,
		repoFactory:   repoFactory,
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
func (rm *ReadModel[T, R]) Start(ctx context.Context) error {
	rm.ctx, rm.cancel = context.WithCancel(ctx)
	rm.pollTicker = time.NewTicker(rm.config.PollInterval)

	rm.wg.Add(1)
	go rm.runWatch()

	return nil
}

// Stop gracefully stops the read model
func (rm *ReadModel[T, R]) Stop() {
	if rm.cancel != nil {
		rm.cancel()
	}
	if rm.pollTicker != nil {
		rm.pollTicker.Stop()
	}
}

// Wait blocks until all goroutines finish and returns any accumulated errors
func (rm *ReadModel[T, R]) Wait() error {
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
func (rm *ReadModel[T, R]) Errors() <-chan error {
	return rm.errCh
}

// IsCaughtUp returns true when the read model has processed all available events.
// Starts false, becomes true after a poll cycle finds no new events.
func (rm *ReadModel[T, R]) IsCaughtUp() bool {
	return rm.caughtUp
}

// runWatch is the Watch polling loop
func (rm *ReadModel[T, R]) runWatch() {
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
func (rm *ReadModel[T, R]) processNextBatch() error {
	batch, err := rm.fetchBatch()
	if err != nil {
		return fmt.Errorf("fetch batch: %w", err)
	}
	if len(batch) == 0 {
		rm.caughtUp = true
		return nil
	}

	_, err = rm.db.Transact(func(tr fdb.Transaction) (any, error) {
		repo := rm.repoFactory(tr, rm.dataSpace)
		var lastVS dcb.Versionstamp
		for _, item := range batch {
			ev, err := rm.eventRegistry.deserialize(item.event)
			if err != nil {
				return nil, fmt.Errorf("deserialize event at %x: %w", item.vs[:], err)
			}
			if err := rm.handler(repo, ev); err != nil {
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
func (rm *ReadModel[T, R]) fetchBatch() ([]vsRawEvent, error) {
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
func (rm *ReadModel[T, R]) fetchRawEvent(tr fdb.ReadTransaction, vs dcb.Versionstamp) (dcb.Event, error) {
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

// waitForCursor blocks until the read model's cursor is >= minVS or ctx is cancelled.
func (rm *ReadModel[T, R]) waitForCursor(ctx context.Context, minVS dcb.Versionstamp) error {
	ticker := time.NewTicker(rm.config.PollInterval)
	defer ticker.Stop()

	for {
		cursor, err := rm.readCursor()
		if err != nil {
			return err
		}
		if cursor != nil && cursor.Compare(minVS) >= 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// readCursor returns the current cursor value, or nil if no cursor set.
func (rm *ReadModel[T, R]) readCursor() (*dcb.Versionstamp, error) {
	var cursor *dcb.Versionstamp
	_, err := rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		data := tr.Get(rm.cursorKey).MustGet()
		if len(data) == 12 {
			var vs dcb.Versionstamp
			copy(vs[:], data)
			cursor = &vs
		}
		return nil, nil
	})
	return cursor, err
}

// currentPosition returns the latest versionstamp across all watched type indexes.
func (rm *ReadModel[T, R]) currentPosition() (*dcb.Versionstamp, error) {
	var maxVS *dcb.Versionstamp
	_, err := rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		maxVS = nil
		for _, typeIndex := range rm.typeIndexes {
			kvs := tr.GetRange(typeIndex, fdb.RangeOptions{Limit: 1, Reverse: true}).GetSliceOrPanic()
			if len(kvs) == 0 {
				continue
			}
			vs := extractVersionstampFromTypeIndex(typeIndex, kvs[0].Key)
			if vs == (dcb.Versionstamp{}) {
				continue
			}
			if maxVS == nil || vs.Compare(*maxVS) > 0 {
				maxVS = &vs
			}
		}
		return nil, nil
	})
	return maxVS, err
}

// Get retrieves values from the read model's data space.
// Waits for cursor to reach current position before querying.
// Returns a slice of pointers; nil entries indicate missing keys.
func (rm *ReadModel[T, R]) Get(ctx context.Context, keys ...Path) ([]*T, error) {
	pos, err := rm.currentPosition()
	if err != nil {
		return nil, err
	}
	if pos != nil {
		if err := rm.waitForCursor(ctx, *pos); err != nil {
			return nil, err
		}
	}

	var results []*T
	_, err = rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		results = make([]*T, len(keys))
		for i, key := range keys {
			data := tr.Get(rm.dataSpace.Pack(pathToTuple(key))).MustGet()
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

// GetByPrefix retrieves all values with keys starting with the given prefix.
// Waits for cursor to reach current position before querying.
func (rm *ReadModel[T, R]) GetByPrefix(ctx context.Context, prefix Path) ([]*T, error) {
	pos, err := rm.currentPosition()
	if err != nil {
		return nil, err
	}
	if pos != nil {
		if err := rm.waitForCursor(ctx, *pos); err != nil {
			return nil, err
		}
	}

	var results []*T
	_, err = rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		prefixSpace := rm.dataSpace.Sub(pathToTuple(prefix)...)
		kvs := tr.GetRange(prefixSpace, fdb.RangeOptions{}).GetSliceOrPanic()
		results = make([]*T, 0, len(kvs))
		for _, kv := range kvs {
			var v T
			if err := json.Unmarshal(kv.Value, &v); err != nil {
				return nil, fmt.Errorf("unmarshal value: %w", err)
			}
			results = append(results, &v)
		}
		return nil, nil
	})
	return results, err
}

// Scan returns all keys with the given prefix (values ignored).
// Waits for cursor to reach current position before querying.
func (rm *ReadModel[T, R]) Scan(ctx context.Context, prefix Path) ([]Path, error) {
	pos, err := rm.currentPosition()
	if err != nil {
		return nil, err
	}
	if pos != nil {
		if err := rm.waitForCursor(ctx, *pos); err != nil {
			return nil, err
		}
	}

	var results []Path
	_, err = rm.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		prefixSpace := rm.dataSpace.Sub(pathToTuple(prefix)...)
		kvs := tr.GetRange(prefixSpace, fdb.RangeOptions{}).GetSliceOrPanic()
		results = make([]Path, 0, len(kvs))
		for _, kv := range kvs {
			keyTuple, err := rm.dataSpace.Unpack(kv.Key)
			if err != nil {
				return nil, err
			}
			path := make(Path, len(keyTuple))
			for i, elem := range keyTuple {
				path[i] = elem.(string)
			}
			results = append(results, path)
		}
		return nil, nil
	})
	return results, err
}

// ReadModelFactory creates a ReadModel from a store
type ReadModelFactory func(store dcb.DcbStore) (ReadModelStarter, error)

// ReadModelStarter is implemented by ReadModel[T]
type ReadModelStarter interface {
	Start(ctx context.Context) error
	Stop()
	Wait() error
	Name() string
	IsCaughtUp() bool
}

// Name returns the read model's name
func (rm *ReadModel[T, R]) Name() string {
	return rm.name
}

// ReadModelRegistry holds registered read model factories
type ReadModelRegistry struct {
	factories []ReadModelFactory
}

// Register adds a read model factory to the registry
func (r *ReadModelRegistry) Register(f ReadModelFactory) {
	r.factories = append(r.factories, f)
}

// StartAll creates and starts all read models, returns stop func
func (r *ReadModelRegistry) StartAll(ctx context.Context, store dcb.DcbStore) (func(), error) {
	var readModels []ReadModelStarter
	seen := make(map[string]bool)

	for _, f := range r.factories {
		rm, err := f(store)
		if err != nil {
			return nil, err
		}
		name := rm.Name()
		if seen[name] {
			return nil, fmt.Errorf("duplicate read model name: %q", name)
		}
		seen[name] = true
		if err := rm.Start(ctx); err != nil {
			return nil, err
		}
		readModels = append(readModels, rm)
	}

	return func() {
		for _, rm := range readModels {
			rm.Stop()
		}
		for _, rm := range readModels {
			_ = rm.Wait()
		}
	}, nil
}
