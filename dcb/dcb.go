package dcb

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"iter"
	"sort"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// Errors
var (
	ErrEmptyEvents           = errors.New("events slice is empty")
	ErrAppendConditionFailed = errors.New("append condition failed")
	ErrInvalidQuery          = errors.New("query must have at least one type or tag")
)

var (
	eventsInTagSubspace = "_e"
)

type DcbStore interface {
	Append(ctx context.Context, events []Event, condition *AppendCondition) error
	Read(ctx context.Context, query Query, opts *ReadOptions) iter.Seq2[StoredEvent, error]
	ReadAll(ctx context.Context) iter.Seq2[StoredEvent, error]
}

// Event represents a single event in the event store
type Event struct {
	Type string
	Tags []string
	Data []byte
}

// Versionstamp is a 12-byte globally unique, monotonically increasing value
type Versionstamp [12]byte

// Compare returns -1 if v < other, 0 if v == other, 1 if v > other
func (v Versionstamp) Compare(other Versionstamp) int {
	return bytes.Compare(v[:], other[:])
}

// String returns hex representation of versionstamp.
func (v Versionstamp) String() string {
	return hex.EncodeToString(v[:])
}

// AppendCondition defines a condition that must be satisfied for an append to succeed
type AppendCondition struct {
	Query Query
	After *Versionstamp // Optional: only check for events strictly AFTER this versionstamp
}

// QueryItem represents a single query clause (types AND tags)
type QueryItem struct {
	Types []string // OR semantics: match any of these types
	Tags  []string // AND semantics: must have all these tags
}

// hasTypesOnly returns true if query has types but no tags
func (q QueryItem) hasTypesOnly() bool {
	return len(q.Types) > 0 && len(q.Tags) == 0
}

// hasTagsOnly returns true if query has tags but no types
func (q QueryItem) hasTagsOnly() bool {
	return len(q.Tags) > 0 && len(q.Types) == 0
}

// hasTypesAndTags returns true if query has both types and tags
func (q QueryItem) hasTypesAndTags() bool {
	return len(q.Types) > 0 && len(q.Tags) > 0
}

// Query represents a union of query items (OR semantics between items)
type Query struct {
	Items []QueryItem
}

// ReadOptions configures how events are read
type ReadOptions struct {
	Limit int           // Maximum number of events to return (0 = unlimited)
	After *Versionstamp // Only return events after this versionstamp (exclusive)
}

// StoredEvent is an event with its assigned position.
type StoredEvent struct {
	Event
	Position Versionstamp
}

// fdbStore provides lock-free event storage with dual-index structure
type fdbStore struct {
	db fdb.Database

	// Subspaces
	events subspace.Subspace // Primary event storage: (versionstamp) -> encoded event
	byType subspace.Subspace // Type index: (type, versionstamp) -> nil
	byTag  subspace.Subspace // Tag tree: (tag1, tag2, ..., type, versionstamp) -> nil

	// Observability
	metrics Metrics
	logger  Logger
}

// NewDcbStore creates a new event store with the given database and namespace
func NewDcbStore(db fdb.Database, namespace string, opts ...func(o *fdbStore)) DcbStore {
	store := newConcreteEventStore(db, namespace)

	for _, oFn := range opts {
		oFn(store)
	}

	return store
}

type StoreOptions struct{}

func (StoreOptions) WithLogger(l Logger) func(s *fdbStore) {
	return func(e *fdbStore) {
		e.logger = l
	}
}

func (StoreOptions) WithMetrics(m Metrics) func(s *fdbStore) {
	return func(e *fdbStore) {
		e.metrics = m
	}
}

// concrete instance is only used in concurrency tests (testing from same package), not exposed publicly
func newConcreteEventStore(db fdb.Database, namespace string) *fdbStore {
	root := subspace.Sub(namespace)
	return &fdbStore{
		db:      db,
		events:  root.Sub("e"),
		byType:  root.Sub("t"),
		byTag:   root.Sub("g"),
		metrics: noopMetrics{},
		logger:  noopLogger{},
	}
}

// rangeAfterVersionstamp creates an FDB range that starts after the given versionstamp
func rangeAfterVersionstamp(ss subspace.Subspace, after Versionstamp) (fdb.Range, error) {
	// Convert 12-byte versionstamp to tuple.Versionstamp
	var txVersion [10]byte
	copy(txVersion[:], after[:10])
	userVersion := binary.BigEndian.Uint16(after[10:12])
	tupleVs := tuple.Versionstamp{TransactionVersion: txVersion, UserVersion: userVersion}

	// Create begin key (exclusive of 'after')
	// Append 0x00 to make it exclusive (next key after the versionstamp)
	beginKey := ss.Pack(tuple.Tuple{tupleVs})
	beginKeyExclusive := append(fdb.Key(beginKey), 0x00)

	// End key is end of subspace
	// Get range keys from subspace (subspace implements fdb.Range)
	_, endKey := ss.FDBRangeKeys()

	return fdb.KeyRange{Begin: beginKeyExclusive, End: endKey}, nil
}

// discoverTypesInTagSubspace discovers all event types under a tag's _events subspace
// Returns list of types found (e.g., ["t1", "t2", "t3"])
func (s fdbStore) discoverTypesInTagSubspace(tr fdb.ReadTransaction, eventsSubspace subspace.Subspace) ([]string, error) {
	var types []string
	typeSet := make(map[string]bool)

	// Scan the _events subspace to find all unique types
	iter := tr.GetRange(eventsSubspace, fdb.RangeOptions{}).Iterator()
	for iter.Advance() {
		kv, err := iter.Get()
		if err != nil {
			return nil, err
		}

		// Extract type from key: (tag1, tag2, ..., _events, type, versionstamp)
		keyTuple, err := s.byTag.Unpack(kv.Key)
		if err != nil {
			return nil, err
		}
		if len(keyTuple) < 2 {
			continue
		}

		eventType, ok := keyTuple[len(keyTuple)-2].(string)
		if !ok {
			continue
		}

		if !typeSet[eventType] {
			typeSet[eventType] = true
			types = append(types, eventType)
		}
	}

	return types, nil
}

// buildQueryRanges constructs FDB ranges for a query item
// For tags queries: returns one range per type (streaming via k-way merge)
func (s fdbStore) buildQueryRanges(tr fdb.ReadTransaction, item QueryItem, after *Versionstamp) ([]fdb.Range, error) {
	// Validate: must have at least one type or tag
	if !item.hasTypesOnly() && !item.hasTagsOnly() && !item.hasTypesAndTags() {
		return nil, ErrInvalidQuery
	}

	var ranges []fdb.Range

	// Case 1: Type-only queries (no tags)
	if item.hasTypesOnly() {
		for _, typ := range item.Types {
			subspace := s.byType.Sub(typ)
			if after != nil {
				r, err := rangeAfterVersionstamp(subspace, *after)
				if err != nil {
					return nil, err
				}
				ranges = append(ranges, r)
			} else {
				ranges = append(ranges, subspace)
			}
		}
		return ranges, nil
	}

	// Case 2: Tags queries (with or without types)
	sortedTags := sortTags(item.Tags)
	subspace := s.byTag
	for _, tag := range sortedTags {
		subspace = subspace.Sub(tag)
	}
	eventsSubspace := subspace.Sub(eventsInTagSubspace)

	var types []string
	if item.hasTypesAndTags() {
		// Types specified - use them directly
		types = item.Types
	} else {
		// Tags-only - discover types
		discoveredTypes, err := s.discoverTypesInTagSubspace(tr, eventsSubspace)
		if err != nil {
			return nil, err
		}
		types = discoveredTypes
	}

	// Create one range per type for k-way merge
	for _, typ := range types {
		typeSubspace := eventsSubspace.Sub(typ)
		if after != nil {
			r, err := rangeAfterVersionstamp(typeSubspace, *after)
			if err != nil {
				return nil, err
			}
			ranges = append(ranges, r)
		} else {
			ranges = append(ranges, typeSubspace)
		}
	}

	return ranges, nil
}

// extractVersionstamp extracts the versionstamp from an index key
// For index keys like (type, _events, versionstamp) or (tag1, tag2, ..., _events, type, versionstamp),
// the versionstamp is the last element in the tuple
func extractVersionstamp(key fdb.Key) Versionstamp {
	// Unpack the tuple to get the versionstamp element
	unpacked, err := tuple.Unpack(key)
	if err != nil || len(unpacked) == 0 {
		return Versionstamp{}
	}

	// Last element is the versionstamp
	vsElem := unpacked[len(unpacked)-1]
	tupleVs, ok := vsElem.(tuple.Versionstamp)
	if !ok {
		return Versionstamp{}
	}

	// Convert tuple.Versionstamp to our Versionstamp type
	var vs Versionstamp
	copy(vs[:10], tupleVs.TransactionVersion[:])
	binary.BigEndian.PutUint16(vs[10:12], tupleVs.UserVersion)

	return vs
}

// generateAllSubsets generates all non-empty subsets of tags in alphabetical order
// Tags are first sorted alphabetically for normalization
// For tags [c, a, b], generates:
//
//	[a]
//	[a, b]
//	[a, b, c]
//	[a, c]
//	[b]
//	[b, c]
//	[c]
func generateAllSubsets(tags []string) [][]string {
	if len(tags) == 0 {
		return nil
	}

	sortedTags := sortTags(tags)

	var result [][]string

	// Generate all non-empty subsets using bit manipulation
	n := len(sortedTags)
	totalSubsets := (1 << n) - 1 // 2^n - 1 (exclude empty set)

	for mask := 1; mask <= totalSubsets; mask++ {
		var subset []string
		for i := range n {
			if mask&(1<<i) != 0 {
				subset = append(subset, sortedTags[i])
			}
		}
		result = append(result, subset)
	}

	return result
}

// Normalize: sort tags alphabetically
func sortTags(tags []string) []string {
	sorted := make([]string, len(tags))
	copy(sorted, tags)
	sort.Strings(sorted)
	return sorted
}
