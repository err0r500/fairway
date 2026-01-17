package dcb_test

import (
	"context"
	"testing"

	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

// Tests for QueryItem helper methods (hasTypesOnly, hasTagsOnly, hasTypesAndTags)

func TestReadEmptyQuery(tt *testing.T) {
	tt.Parallel()

	// Given - store with events
	ctx := context.Background()
	store := dcb.SetupTestStore(tt)

	// When - read with empty query item (no types, no tags)
	var events []dcb.StoredEvent
	var lastErr error
	for event, err := range store.Read(ctx, dcb.Query{Items: []dcb.QueryItem{{}}}, nil) {
		if err != nil {
			lastErr = err
			break
		}
		events = append(events, event)
	}

	// Then - fails with ErrInvalidQuery
	assert.ErrorIs(tt, lastErr, dcb.ErrInvalidQuery)
}

func TestReadQueryWithEmptyTypes(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - store with events
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		tag := dcb.RandomEventTag(t)
		events := dcb.RandomEvents(t)
		setEventsTags(events, []string{tag})
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		// When - read with empty types array but valid tags
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{}, Tags: []string{tag}}}}, nil))

		// Then - returns events with tag (tags-only query)
		assert.Len(t, storedEvents, len(events))
		for _, e := range storedEvents {
			assert.Contains(t, e.Event.Tags, tag)
		}
	})
}

func TestReadQueryWithEmptyTags(tt *testing.T) {
	tt.Parallel()
	rapid.Check(tt, func(t *rapid.T) {
		// Given - store with events
		ctx := context.Background()
		store := dcb.SetupTestStore(tt)
		eventType := dcb.RandomEventType(t)
		events := dcb.RandomEvents(t)
		setEventsType(events, eventType)
		err := store.Append(ctx, events, nil)
		assert.NoError(t, err)

		// When - read with empty tags array but valid types
		storedEvents := dcb.CollectEvents(tt, store.Read(ctx,
			dcb.Query{Items: []dcb.QueryItem{{Types: []string{eventType}, Tags: []string{}}}}, nil))

		// Then - returns events of type (types-only query)
		assert.Len(t, storedEvents, len(events))
		for _, e := range storedEvents {
			assert.Equal(t, eventType, e.Event.Type)
		}
	})
}

func TestVersionstampCompare(tt *testing.T) {
	tt.Parallel()

	// Given
	vs1 := dcb.Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}
	vs2 := dcb.Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0}
	vs3 := dcb.Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}

	// When/Then
	assert.Equal(tt, -1, vs1.Compare(vs2)) // vs1 < vs2
	assert.Equal(tt, 1, vs2.Compare(vs1))  // vs2 > vs1
	assert.Equal(tt, 0, vs1.Compare(vs3))  // vs1 == vs3
}

func TestVersionstampString(tt *testing.T) {
	tt.Parallel()

	// Given
	vs := dcb.Versionstamp{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c}

	// When
	str := vs.String()

	// Then
	assert.Equal(tt, "0102030405060708090a0b0c", str)
}

// ============================================================================
// INTEGRATION - Append then Read
// ============================================================================

// func TestAppendThenReadBack(tt *testing.T) {
// 	tt.Parallel()
// 	rapid.Check(tt, func(t *rapid.T) {
// 		// Given - empty store
// 		store := setupTestStore(tt)

// 		eventType := randomEventType(t)
// 		events := randomEventsOfType(t, 10, eventType)

// 		// When - append then read
// 		err := store.Append(context.Background(), events, nil)
// 		assert.NoError(t, err)

// 		stored := collectEvents(tt, store.Read(context.Background(), Query{Items: []QueryItem{{Types: []string{eventType}}}}, nil))

// 		// Then - all events returned with assigned versionstamps
// 		assert.Len(t, stored, len(events))
// 		for _, e := range stored {
// 			assert.NotNil(t, e.Pos)
// 		}
// 	})
// }

// func TestVersionstampsMonotonic(tt *testing.T) {
// 	tt.Parallel()
// 	rapid.Check(tt, func(t *rapid.T) {
// 		// Given - empty store
// 		store := setupTestStore(tt)

// 		eventType := randomEventType(t)

// 		// When - append in separate calls
// 		for i := 0; i < 3; i++ {
// 			events := randomEventsOfType(t, 2, eventType)
// 			err := store.Append(context.Background(), events, nil)
// 			assert.NoError(t, err)
// 		}

// 		// Then - versionstamps are monotonic increasing
// 		stored := collectEvents(tt, store.Read(context.Background(), Query{Items: []QueryItem{{Types: []string{eventType}}}}, nil))
// 		assert.True(t, eventsAreOrdered(stored))
// 	})
// }

// func TestEventDataRoundtrip(tt *testing.T) {
// 	tt.Parallel()
// 	rapid.Check(tt, func(t *rapid.T) {
// 		// Given - event with specific data
// 		store := setupTestStore(tt)

// 		data := randomEventData(t)
// 		event := Event{
// 			Type: randomEventType(t),
// 			Data: data,
// 		}

// 		// When - append then read
// 		err := store.Append(context.Background(), []Event{event}, nil)
// 		assert.NoError(t, err)

// 		stored := collectEvents(tt, store.Read(context.Background(), Query{Items: []QueryItem{{Types: []string{event.Type}}}}, nil))

// 		// Then - data matches exactly
// 		assert.Len(t, stored, 1)
// 		assert.Equal(t, data, stored[0].Data)
// 	})
// }

// func TestTagsRoundtrip(tt *testing.T) {
// 	tt.Parallel()
// 	rapid.Check(tt, func(t *rapid.T) {
// 		// Given - event with tags
// 		store := setupTestStore(tt)

// 		tags := randomEventTags(t)
// 		if len(tags) == 0 {
// 			t.Skip("need at least 1 tag")
// 		}

// 		event := Event{
// 			Type: randomEventType(t),
// 			Tags: tags,
// 		}

// 		// When - append then read
// 		err := store.Append(context.Background(), []Event{event}, nil)
// 		assert.NoError(t, err)

// 		stored := collectEvents(tt, store.Read(context.Background(), Query{Items: []QueryItem{{Types: []string{event.Type}}}}, nil))

// 		// Then - tags match exactly
// 		assert.Len(t, stored, 1)
// 		assert.Equal(t, tags, stored[0].Tags)
// 	})
// }

// // ============================================================================
// // HELPERS
// // ============================================================================

// func randomEventTags(t *rapid.T) []string {
// 	count := rapid.IntRange(0, 3).Draw(t, "tagCount")
// 	if count == 0 {
// 		return nil
// 	}
// 	tags := make([]string, count)
// 	for i := 0; i < count; i++ {
// 		tags[i] = randomEventTag(t)
// 	}
// 	return tags
// }

// func randomEventData(t *rapid.T) []byte {
// 	data := rapid.SliceOfN(rapid.Byte(), 0, 100).Draw(t, "data")
// 	if data == nil {
// 		return []byte{}
// 	}
// 	return data
// }

// func randomEvents(t *rapid.T, maxCount int) []Event {
// 	count := rapid.IntRange(2, maxCount).Draw(t, "eventCount")
// 	events := make([]Event, count)
// 	for i := 0; i < count; i++ {
// 		events[i] = Event{
// 			Type: randomEventType(t),
// 			Tags: randomEventTags(t),
// 			Data: randomEventData(t),
// 		}
// 	}
// 	return events
// }

// func randomEventsOfType(t *rapid.T, maxCount int, eventType string) []Event {
// 	count := rapid.IntRange(1, maxCount).Draw(t, "eventCount")
// 	events := make([]Event, count)
// 	for i := 0; i < count; i++ {
// 		events[i] = Event{
// 			Type: eventType,
// 			Tags: randomEventTags(t),
// 			Data: randomEventData(t),
// 		}
// 	}
// 	return events
// }

// func randomEventsExcludingType(t *rapid.T, maxCount int, excludeType string) []Event {
// 	count := rapid.IntRange(1, maxCount).Draw(t, "eventCount")
// 	events := make([]Event, count)
// 	for i := 0; i < count; i++ {
// 		eventType := randomEventType(t)
// 		for eventType == excludeType {
// 			eventType = randomEventType(t)
// 		}
// 		events[i] = Event{
// 			Type: eventType,
// 			Tags: randomEventTags(t),
// 			Data: randomEventData(t),
// 		}
// 	}
// 	return events
// }

// func randomEventsWithTag(t *rapid.T, maxCount int, tag string) []Event {
// 	count := rapid.IntRange(1, maxCount).Draw(t, "eventCount")
// 	events := make([]Event, count)
// 	for i := 0; i < count; i++ {
// 		events[i] = Event{
// 			Type: randomEventType(t),
// 			Tags: []string{tag},
// 			Data: randomEventData(t),
// 		}
// 	}
// 	return events
// }

// func setupTestStore(t *testing.T) *fdbdcb.EventStore {
// 	t.Helper()

// 	fdb.MustAPIVersion(740)
// 	db := fdb.MustOpenDefault()

// 	// Use unique namespace per test
// 	namespace := fmt.Sprintf("t-%d", uuid.New())
// 	store := fdbdcb.NewEventStore(db, namespace)

// 	// Clean up after test
// 	t.Cleanup(func() {
// 		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
// 			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(namespace), End: fdb.Key(namespace + "\xff")})
// 			return nil, nil
// 		})
// 	})

// 	return store
// }
