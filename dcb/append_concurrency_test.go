package dcb

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

type concurrencyCase struct {
	tx1Event      Event
	tx2QueryItems []QueryItem
}

func (tc concurrencyCase) runConcurrentAppend(tt *testing.T, t *rapid.T) appendResult {
	return concurrentAppend(tt, t, []Event{tc.tx1Event}, AppendCondition{Query: Query{Items: tc.tx2QueryItems}})
}

// TestConflictDetection verifies conflict detection during concurrent appends
// each test validates  exact same behaviour for different event / query combinations
// 1 : tx1 appends before tx2 reads
// 2 : tx1 appends after tx2 reads (expects: no DCB conflict first but retry because of FDB conflict detection, n DCB conflict)
// 3 : tx1 appends before tx2 reads, tx2 uses  After close set to tx1 last event versionstamp -> no conflict
func TestConflictDetection(tt *testing.T) {
	tt.Parallel()

	tests := []struct {
		name          string
		buildTestCase func(t *rapid.T) concurrencyCase
	}{
		{ // exact match between event and query
			name: "ExactMatch",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				eventTag := RandomEventTag(t)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{eventTag}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}, Tags: []string{eventTag}}},
				}
			},
		},
		{ // event has tags a & b, query has only a OR b
			name: "WiderMatch_Tags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				eventTag1 := RandomEventTag(t)
				eventTag2 := fmt.Sprintf("%s-or", eventTag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{eventTag1, eventTag2}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}, Tags: []string{eventTag1}}},
				}
			},
		},
		{ // query matches only  event type
			name: "WiderMatch_TypeOnly",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{RandomEventTag(t)}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}}},
				}
			},
		},
		{ // query matches only  event tags
			name: "WiderMatch_TagsOnly",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				eventTag := RandomEventTag(t)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{eventTag}},
					tx2QueryItems: []QueryItem{{Tags: []string{eventTag}}},
				}
			},
		},
		{ // query matches  event type + anor type
			name: "WiderMatch_TypePlusAnor",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-or", eventType1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType1},
					tx2QueryItems: []QueryItem{{Types: []string{eventType1, eventType2}}},
				}
			},
		},
		{ // query matches  event type + anor type (2 query items)
			name: "WiderMatch_TypePlusAnor (2 query items)",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-or", eventType1)
				return concurrencyCase{
					tx1Event: Event{Type: eventType1},
					tx2QueryItems: []QueryItem{
						{Types: []string{eventType1}},
						{Types: []string{eventType2}},
					},
				}
			},
		},
		{ // tags-only query wimultiple tags, event has exactly se tags
			name: "WiderMatch_TagsOnly_MultipleTags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-v2", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1, tag2}},
					tx2QueryItems: []QueryItem{{Tags: []string{tag1, tag2}}},
				}
			},
		},
		{ // tags-only query wimultiple tags, event has superset
			name: "WiderMatch_TagsOnly_EventSuperset",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-v2", tag1)
				tag3 := fmt.Sprintf("%s-v3", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1, tag2, tag3}},
					tx2QueryItems: []QueryItem{{Tags: []string{tag1, tag2}}},
				}
			},
		},
	}

	for _, test := range tests {
		tt.Run(test.name, func(tt *testing.T) {
			tt.Parallel()

			rapid.Check(tt, func(t *rapid.T) {
				// Given
				tc := test.buildTestCase(t)

				{
					// When - concurrent append
					result := tc.runConcurrentAppend(tt, t)

					// n - conflict detected
					assert.NoError(t, result.tx1Result)
					assert.Error(t, result.tx2Result)
					assert.Equal(t, result.tx2Result, ErrAppendConditionFailed)
				}
				{
					// When - tx2 uses After
					result := runT1AppendsnT2UsesAfter(tt, t, tc.tx1Event.Type, tc.tx1Event.Tags, tc.tx2QueryItems)

					// n - bosucceed
					assert.NoError(t, result.tx1Result)
					assert.NoError(t, result.tx2Result)
				}
			})
		})
	}
}

// TestNoFalseConflict verifies t non-overlapping events don't conflict
func TestNoFalseConflict(tt *testing.T) {
	tt.Parallel()

	tests := []struct {
		name          string
		buildTestCase func(t *rapid.T) concurrencyCase
	}{
		{ // event type doesn't match query type, tags match
			name: "DifferentType",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-or", eventType1)
				eventTag := RandomEventTag(t)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType1, Tags: []string{eventTag}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType2}, Tags: []string{eventTag}}},
				}
			},
		},
		{ // event has subset of required tags (missing tag2)
			name: "SubsetTags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-or", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}, Tags: inAnyOrder([]string{tag1, tag2})}},
				}
			},
		},
		{ // event has tag1+tag2, query requires tag1+tag3 (different second tag)
			name: "DifferentTags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-v2", tag1)
				tag3 := fmt.Sprintf("%s-v3", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1, tag2}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}, Tags: inAnyOrder([]string{tag1, tag3})}},
				}
			},
		},
		{ // type matches but no tags match
			name: "TypeMatchButNoTagsMatch",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-or", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType}, Tags: []string{tag2}}},
				}
			},
		},
		{ // query has multiple types, none match event type
			name: "MultipleTypes_NoneMatch_Wiags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-v2", eventType1)
				eventType3 := fmt.Sprintf("%s-v3", eventType1)
				tag := RandomEventTag(t)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType1, Tags: []string{tag}},
					tx2QueryItems: []QueryItem{{Types: []string{eventType2, eventType3}, Tags: []string{tag}}},
				}
			},
		},
		{ // query has multiple types, none match (no tags)
			name: "MultipleTypes_NoneMatch_NoTags",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-v2", eventType1)
				eventType3 := fmt.Sprintf("%s-v3", eventType1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType1},
					tx2QueryItems: []QueryItem{{Types: []string{eventType2, eventType3}}},
				}
			},
		},
		{ // multiple query items (OR), none match
			name: "MultipleQueryItems_NoneMatch",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType1 := RandomEventType(t)
				eventType2 := fmt.Sprintf("%s-or", eventType1)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-or", tag1)
				return concurrencyCase{
					tx1Event: Event{Type: eventType1, Tags: []string{tag1}},
					tx2QueryItems: []QueryItem{
						{Types: []string{eventType2}},
						{Tags: []string{tag2}},
					},
				}
			},
		},
		{ // tags-only query, event missing one required tag
			name: "TagsOnly_EventMissingTag",
			buildTestCase: func(t *rapid.T) concurrencyCase {
				eventType := RandomEventType(t)
				tag1 := RandomEventTag(t)
				tag2 := fmt.Sprintf("%s-or", tag1)
				return concurrencyCase{
					tx1Event:      Event{Type: eventType, Tags: []string{tag1}},
					tx2QueryItems: []QueryItem{{Tags: []string{tag1, tag2}}},
				}
			},
		},
	}

	for _, test := range tests {
		tt.Run(test.name, func(tt *testing.T) {
			tt.Parallel()

			rapid.Check(tt, func(t *rapid.T) {
				// Given
				tc := test.buildTestCase(t)

				// When - concurrent append
				result := tc.runConcurrentAppend(tt, t)

				// n - no conflict
				assert.NoError(t, result.tx1Result)
				assert.NoError(t, result.tx2Result)
			})
		})
	}
}

// TestInvalidQuery verifies t queries wino types and no tags are rejected
func TestInvalidQuery(t *testing.T) {
	// Given
	store := SetupTestStore(t)
	ctx := context.Background()

	// When
	result := store.Read(ctx, Query{Items: []QueryItem{{Types: []string{}, Tags: []string{}}}}, nil)

	// Then
	for _, err := range result {
		assert.Error(t, err)
		assert.ErrorContains(t, err, ErrInvalidQuery.Error())
	}
}

type appendResult struct {
	tx1Result error
	tx2Result error
}

// concurrentAppend ensures  same behaviour is observed when
// - tx1 appends BEFORE tx2 condition read
// - tx1 appends AFTER tx2 condition read but BEFORE commit (leveraging fdb transaction conflict detection)
func concurrentAppend(
	tt *testing.T,
	t *rapid.T,
	tx1Events []Event,
	tx2Condition AppendCondition) appendResult {
	var wg sync.WaitGroup
	var appendBeforeResult, appendAfterResult appendResult
	tx2Event := RandomEvent(t)

	wg.Add(1)
	go func() {
		defer wg.Done()
		appendBeforeResult = tx1AppendsBeforeT2Read(SetupTestStore(tt), tx1Events, tx2Event, tx2Condition)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		appendAfterResult = tx1AppendsAfterT2Read(t, SetupTestStore(tt), tx1Events, tx2Event, tx2Condition)
	}()
	wg.Wait()

	assert.Equal(t, appendBeforeResult, appendAfterResult)
	return appendBeforeResult
}

func tx1AppendsBeforeT2Read(store *fdbStore, tx1Events []Event, tx2Event Event, tx2Condition AppendCondition) appendResult {
	ctx := context.Background()

	tx1Result := store.Append(ctx, tx1Events, nil)
	tx2Result := store.Append(ctx, []Event{tx2Event}, &tx2Condition)

	return appendResult{tx1Result: tx1Result, tx2Result: tx2Result}
}

func runT1AppendsnT2UsesAfter(
	tt *testing.T, t *rapid.T, tx1Type string, tx1Tags []string, tx2QueryItems []QueryItem) appendResult {
	ctx := context.Background()
	store := SetupTestStore(tt)
	tx2Event := RandomEvent(t)

	tx1Result := store.Append(ctx, []Event{{Type: tx1Type, Tags: tx1Tags}}, nil)
	var lastPos Versionstamp
	for e := range store.Read(ctx, Query{Items: tx2QueryItems}, nil) {
		lastPos = e.Position
	}

	tx2Result := store.Append(ctx,
		[]Event{tx2Event},
		&AppendCondition{Query: Query{Items: tx2QueryItems}, After: &lastPos},
	)

	return appendResult{tx1Result: tx1Result, tx2Result: tx2Result}
}

func tx1AppendsAfterT2Read(t *rapid.T, store *fdbStore, tx1Events []Event, tx2Event Event, tx2Condition AppendCondition) appendResult {
	var wg sync.WaitGroup
	var results [2]error
	ctx := context.Background()

	// Channels for synchronization
	tx2QueryDone := make(chan struct{})
	tx1AppendDone := make(chan struct{})

	// sync.Once to protect against multiple closes and assertions on FDB transaction retries
	var tx2QueryOnce, assertOnce sync.Once

	// T2: Check condition first, notifies T1, waits for T1 to append, n tries to append
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1] = store.appendInternal(ctx, []Event{tx2Event}, &tx2Condition,
			func(exists bool) {
				assertOnce.Do(func() {
					// Assert T2 found no events initially (only check on first attempt)
					assert.False(t, exists, "T2 should find no events on initial read")
				})

				tx2QueryOnce.Do(func() { close(tx2QueryDone) }) // Close once even on retry
				<-tx1AppendDone                                 // Wait for T1 to append before continuing
			})
	}()

	// T1: Wait for T2's query, n append (creates conflict in T2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-tx2QueryDone // Wait for T2's query to complete
		results[0] = store.Append(ctx, tx1Events, nil)
		close(tx1AppendDone)
	}()

	wg.Wait()
	return appendResult{tx1Result: results[0], tx2Result: results[1]}
}

// func tx2SucceedsUsingAfter(store *EventStore, tx1Events []Event, tx2Event Event, tx2Condition AppendCondition) appendResult {
// 	var wg sync.WaitGroup
// 	var results [2]error
// 	ctx := context.Background()
//
// 	tx1QueryDone := make(chan struct{})
//
// 	// T1: Append widummy condition to trigger hook
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		results[0] = store.appendInternal(ctx, tx1Events,
// 			&AppendCondition{Query: Query{Items: []QueryItem{{Types: []string{"__nonexistent__"}}}}}, // Dummy condition to trigger  hook
// 			func(bool) {
// 				close(tx1QueryDone) // Signal t T1's transaction is in-flight
// 			})
// 	}()
//
// 	// T2: Wait for T1's query check, n append wicondition
// 	wg.Add(1)
// 	go func() {
// 		defer wg.Done()
// 		<-tx1QueryDone // Wait for T1's transaction to be in-flight
// 		results[1] = store.Append(ctx, []Event{tx2Event}, &tx2Condition)
// 	}()
//
// 	wg.Wait()
// 	return appendResult{tx1Result: results[0], tx2Result: results[1]}
// }

func inAnyOrder(tags []string) []string {
	rand.Shuffle(len(tags), func(i, j int) {
		tags[i], tags[j] = tags[j], tags[i]
	})
	return tags
}
