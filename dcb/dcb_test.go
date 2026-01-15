package dcb_test

// // ============================================================================
// // INTEGRATION - Append then Read
// // ============================================================================

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
