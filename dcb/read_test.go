package dcb_test

import (
	"context"
	"testing"

	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// ============================================================================
// READ - Query Filtering
// ============================================================================

func TestReadByType(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events of type T1 and T2
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		type1 := dcb.RandomEventType(t)
		events1 := dcb.RandomEvents(t)
		setEventsType(events1, type1)

		eventsWithOtherType := dcb.RandomEvents(t)
		setEventsType(eventsWithOtherType, type1+"_other")

		err := store.Append(ctx, append(events1, eventsWithOtherType...), nil)
		assert.NoError(t, err)

		// When - read Query{types:[T1]}
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{type1}}}}, nil))

		// Then - returns only T1 events
		assert.Len(t, storedEvents, len(events1))
		assert.ElementsMatch(t, events1, toEvents(storedEvents))
		for _, e := range storedEvents {
			assert.Equal(t, type1, e.Event.Type)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadByMultipleTypes(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events of types T1, T2, T3
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		type1 := dcb.RandomEventType(t)
		type2 := type1 + "_other"
		type3 := type1 + "_other2"

		events1 := dcb.RandomEvents(t)
		setEventsType(events1, type1)

		events2 := dcb.RandomEvents(t)
		setEventsType(events2, type2)

		events3 := dcb.RandomEvents(t)
		setEventsType(events3, type3)

		err := store.Append(ctx, append(append(events1, events2...), events3...), nil)
		assert.NoError(t, err)

		// When - read Query{types:[T1,T3]} (OR semantics)
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{type1, type3}}}}, nil))

		// Then - returns T1 and T3 events
		expectedEvents := append(events1, events3...)
		assert.Len(t, storedEvents, len(expectedEvents))
		assert.ElementsMatch(t, expectedEvents, toEvents(storedEvents))
		for _, e := range storedEvents {
			assert.True(t, e.Event.Type == type1 || e.Event.Type == type3)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadByTags(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events with tags A and B
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		tagA := dcb.RandomEventTag(t)
		tagB := tagA + "_other"

		eventsA := dcb.RandomEvents(t)
		setEventsTags(eventsA, []string{tagA})

		eventsB := dcb.RandomEvents(t)
		setEventsTags(eventsB, []string{tagB})

		err := store.Append(ctx, append(eventsA, eventsB...), nil)
		assert.NoError(t, err)

		// When - read Query{tags:[A]}
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Tags: []string{tagA}}}}, nil))

		// Then - returns only events with tag A
		assert.Len(t, storedEvents, len(eventsA))
		for _, e := range storedEvents {
			assert.Contains(t, e.Event.Tags, tagA)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadByMultipleTags(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events with different tag combinations
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		tagA := dcb.RandomEventTag(t)
		tagB := tagA + "_other"

		// Events with both A and B
		eventsAB := dcb.RandomEvents(t)
		setEventsTags(eventsAB, []string{tagA, tagB})

		// Events with only A
		eventsA := dcb.RandomEvents(t)
		setEventsTags(eventsA, []string{tagA})

		err := store.Append(ctx, append(eventsAB, eventsA...), nil)
		assert.NoError(t, err)

		// When - read Query{tags:[A,B]} (AND semantics - must have both)
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Tags: []string{tagA, tagB}}}}, nil))

		// Then - returns only events with both tags
		assert.Len(t, storedEvents, len(eventsAB))
		for _, e := range storedEvents {
			assert.Contains(t, e.Event.Tags, tagA)
			assert.Contains(t, e.Event.Tags, tagB)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadByTypeAndTags(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events with different type/tag combinations
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		type1 := dcb.RandomEventType(t)
		type2 := type1 + "_other"
		tagA := dcb.RandomEventTag(t)
		tagB := tagA + "_other"

		// E1(T1, A), E2(T1, B), E3(T2, A)
		e1 := dcb.RandomEvents(t)
		setEventsType(e1, type1)
		setEventsTags(e1, []string{tagA})

		e2 := dcb.RandomEvents(t)
		setEventsType(e2, type1)
		setEventsTags(e2, []string{tagB})

		e3 := dcb.RandomEvents(t)
		setEventsType(e3, type2)
		setEventsTags(e3, []string{tagA})

		err := store.Append(ctx, append(append(e1, e2...), e3...), nil)
		assert.NoError(t, err)

		// When - read Query{types:[T1], tags:[A]}
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{type1}, Tags: []string{tagA}}}}, nil))

		// Then - returns only E1
		assert.Len(t, storedEvents, len(e1))
		assert.ElementsMatch(t, e1, toEvents(storedEvents))
		for _, e := range storedEvents {
			assert.Equal(t, type1, e.Event.Type)
			assert.Contains(t, e.Event.Tags, tagA)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadMultipleQueryItems(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events with different characteristics
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		type1 := dcb.RandomEventType(t)
		tagA := dcb.RandomEventTag(t)

		// E1(T1), E2(tag:A), E3(other)
		e1 := dcb.RandomEvents(t)
		setEventsType(e1, type1)

		e2 := dcb.RandomEvents(t)
		setEventsTags(e2, []string{tagA})

		e3 := dcb.RandomEvents(t)

		err := store.Append(ctx, append(append(e1, e2...), e3...), nil)
		assert.NoError(t, err)

		// When - read Query{items:[{types:[T1]}, {tags:[A]}]} (OR between items)
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx, dcb.Query{Items: []dcb.QueryItem{
			{Types: []string{type1}},
			{Tags: []string{tagA}},
		}}, nil))

		// Then - returns E1 and E2
		assert.GreaterOrEqual(t, len(storedEvents), len(e1)+len(e2))
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadEmptyResult(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events of type T1
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		type1 := dcb.RandomEventType(t)
		type2 := type1 + "_other"

		events := dcb.RandomEvents(t)
		setEventsType(events, type1)
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		// When - read Query{types:[T2]}
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{type2}}}}, nil))

		// Then - returns empty
		assert.Empty(t, storedEvents)
	})
}

// ============================================================================
// READ - Options
// ============================================================================

func TestReadWithLimit(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - many events stored
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		eventType := dcb.RandomEventType(t)
		events := dcb.RandomEvents(t)
		setEventsType(events, eventType)
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		limit := rapid.IntRange(1, 10).Draw(t, "limit")

		// When - read with Limit
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}},
			&dcb.ReadOptions{Limit: limit}))

		// Then - returns at most limit events
		assert.LessOrEqual(t, len(storedEvents), limit)
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadWithAfter(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - events stored
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		eventType := dcb.RandomEventType(t)
		events := dcb.RandomEvents(t)
		setEventsType(events, eventType)
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		// Get all events to find a midpoint
		all := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}}, nil))
		if len(all) < 2 {
			t.Skip("need at least 2 events")
		}

		midpoint := all[len(all)/2].Position

		// When - read with After
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}},
			&dcb.ReadOptions{After: &midpoint}))

		// Then - returns only events after midpoint
		for _, e := range storedEvents {
			assert.True(t, e.Position.Compare(midpoint) > 0)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func TestReadWithLimitAndAfter(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - many events stored
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		eventType := dcb.RandomEventType(t)
		events := dcb.RandomEvents(t)
		setEventsType(events, eventType)
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		// Get all to find midpoint
		all := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}}, nil))
		if len(all) < 5 {
			t.Skip("need at least 5 events")
		}

		midpoint := all[2].Position
		limit := 3

		// When - read with After and Limit
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}},
			&dcb.ReadOptions{After: &midpoint, Limit: limit}))

		// Then - returns at most limit events after midpoint
		assert.LessOrEqual(t, len(storedEvents), limit)
		for _, e := range storedEvents {
			assert.True(t, e.Position.Compare(midpoint) > 0)
		}
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func setEventsType(events []dcb.Event, eventType string) {
	for i := range events {
		events[i].Type = eventType
	}
}

func setEventsTags(events []dcb.Event, tags []string) {
	for i := range events {
		events[i].Tags = tags
	}
}
