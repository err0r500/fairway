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
		err := store.Append(context.Background(), []dcb.Event{event}, nil)

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
		err := store.Append(ctx, events, nil)

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
	err := store.Append(context.Background(), []dcb.Event{}, nil)

	// Then - fails with ErrEmptyEvents
	assert.ErrorIs(tt, err, dcb.ErrEmptyEvents)
}
