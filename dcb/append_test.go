package dcb_test

import (
	"context"
	"testing"

	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

func TestAppendSingleEvent(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given
		store := dcb.SetupTestStore(tt)
		event := dcb.RandomEvent(t)

		// When
		err := store.Append(context.Background(), []dcb.Event{event})

		// Then - succeeds
		assert.NoError(t, err)
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(context.Background()))
		assert.Len(t, storedEvents, 1)
		assert.Equal(t, event, storedEvents[0].Event)
	})
}

func TestAppendMultipleEvents(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		ctx := context.Background()
		// Given - empty store
		store := dcb.SetupTestStore(tt)
		events := dcb.RandomEvents(t)

		// When - append multiple events in single call
		err := store.Append(ctx, events)

		// Then - succeeds, all stored in order
		assert.NoError(t, err)
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.ElementsMatch(t, events, toEvents(storedEvents))
		assert.True(t, dcb.EventsAreStriclyOrdered(storedEvents))
	})
}

func toEvents(storedEvents []dcb.StoredEvent) []dcb.Event {
	events := []dcb.Event{}
	for i := range storedEvents {
		events = append(events, storedEvents[i].Event)
	}
	return events
}

func TestAppendEmptySlice(tt *testing.T) {
	tt.Parallel()

	// Given - empty store
	store := dcb.SetupTestStore(tt)

	// When - append empty slice
	err := store.Append(context.Background(), []dcb.Event{})

	// Then - fails with ErrEmptyEvents
	assert.ErrorIs(tt, err, dcb.ErrEmptyEvents)
}

func TestAppendEventWithNoTags(tt *testing.T) {
	tt.Parallel()

	// Given
	store := dcb.SetupTestStore(tt)
	event := dcb.Event{
		Type: "test_event",
		Tags: nil, // No tags
		Data: []byte("test"),
	}

	// When - append event with no tags
	err := store.Append(context.Background(), []dcb.Event{event})

	// Then - succeeds and event is stored without tags
	assert.NoError(tt, err)
	storedEvents := dcb.CollectEvents(tt, store.ReadAll(context.Background()))
	assert.Len(tt, storedEvents, 1)
	assert.Empty(tt, storedEvents[0].Event.Tags) // Can be nil or empty slice
	assert.Equal(tt, event.Type, storedEvents[0].Event.Type)
}

func TestAppendConditionExists(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - event already exists
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType := dcb.RandomEventType(t)
		existing := dcb.RandomEvent(t)
		existing.Type = eventType

		err := store.Append(ctx, []dcb.Event{existing})
		assert.NoError(t, err)

		// When - try to append with condition that event doesn't exist
		newEvent := dcb.RandomEvent(t)
		condition := &dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}},
		}

		err = store.Append(ctx, []dcb.Event{newEvent}, *condition)

		// Then - fails with ErrAppendConditionFailed
		assert.ErrorIs(t, err, dcb.ErrAppendConditionFailed)

		// Verify only original event exists
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
		assert.Equal(t, existing, storedEvents[0].Event)
	})
}

func TestAppendConditionDoesNotExist(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - empty store
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType := dcb.RandomEventType(t)

		// When - append with condition that event doesn't exist
		newEvent := dcb.RandomEvent(t)
		condition := &dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}}}},
		}

		err := store.Append(ctx, []dcb.Event{newEvent}, *condition)

		// Then - succeeds
		assert.NoError(t, err)
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
	})
}

func TestAppendMultipleConditions_AllPass(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - empty store
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType1 := dcb.RandomEventType(t)
		eventType2 := eventType1 + "_other"

		// When - append with multiple conditions, all pass (no existing events)
		newEvent := dcb.RandomEvent(t)
		cond1 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType1}}}},
		}
		cond2 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType2}}}},
		}

		err := store.Append(ctx, []dcb.Event{newEvent}, cond1, cond2)

		// Then - succeeds
		assert.NoError(t, err)
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
	})
}

func TestAppendMultipleConditions_OneFails(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - store with one event
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType1 := dcb.RandomEventType(t)
		eventType2 := eventType1 + "_other"

		existing := dcb.RandomEvent(t)
		existing.Type = eventType1
		err := store.Append(ctx, []dcb.Event{existing})
		assert.NoError(t, err)

		// When - append with multiple conditions, one fails
		newEvent := dcb.RandomEvent(t)
		cond1 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType1}}}}, // fails
		}
		cond2 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType2}}}}, // passes
		}

		err = store.Append(ctx, []dcb.Event{newEvent}, cond1, cond2)

		// Then - fails with ErrAppendConditionFailed
		assert.ErrorIs(t, err, dcb.ErrAppendConditionFailed)

		// Verify only original event exists
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
		assert.Equal(t, existing, storedEvents[0].Event)
	})
}

func TestAppendMultipleConditions_SecondFails(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - store with one event
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType1 := dcb.RandomEventType(t)
		eventType2 := eventType1 + "_other"

		existing := dcb.RandomEvent(t)
		existing.Type = eventType2
		err := store.Append(ctx, []dcb.Event{existing})
		assert.NoError(t, err)

		// When - append with multiple conditions, second one fails
		newEvent := dcb.RandomEvent(t)
		cond1 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType1}}}}, // passes
		}
		cond2 := dcb.AppendCondition{
			Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType2}}}}, // fails
		}

		err = store.Append(ctx, []dcb.Event{newEvent}, cond1, cond2)

		// Then - fails with ErrAppendConditionFailed
		assert.ErrorIs(t, err, dcb.ErrAppendConditionFailed)

		// Verify only original event exists
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
		assert.Equal(t, existing, storedEvents[0].Event)
	})
}

func TestAppendNoConditions(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - empty store
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)

		// When - append without any conditions (variadic empty)
		event := dcb.RandomEvent(t)
		err := store.Append(ctx, []dcb.Event{event})

		// Then - succeeds
		assert.NoError(t, err)
		storedEvents := dcb.CollectEvents(tt, store.ReadAll(ctx))
		assert.Len(t, storedEvents, 1)
	})
}
