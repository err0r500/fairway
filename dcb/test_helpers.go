//go:build test

package dcb

import (
	"fmt"
	"testing"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"pgregory.net/rapid"
)

func SetupTestStore(t *testing.T) *fdbStore {
	t.Helper()

	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()

	// Use unique namespace per test
	namespace := fmt.Sprintf("t-%d", uuid.New())
	store := newConcreteEventStore(db, namespace)

	// Clean up after test
	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(namespace), End: fdb.Key(namespace + "\xff")})
			return nil, nil
		})
	})

	return store
}

func RandomEventType(t *rapid.T) string {
	return randomEventTypeGen().Draw(t, "eventType")
}

func RandomEvent(t *rapid.T) Event {
	return randomEventGen().Draw(t, "event")
}
func RandomEvents(t *rapid.T) []Event {
	return randomEventsGen().Draw(t, "events")
}

func RandomEventTag(t *rapid.T) string {
	return randomEventTagGen().Draw(t, "eventTag")
}

// GENERATORS
func randomEventTypeGen() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"item_updated", "task_created", "order_placed", "user_registered"})
}

func randomEventTagGen() *rapid.Generator[string] {
	return rapid.SampledFrom([]string{"list:1", "list:2", "user:123", "user:456", "tenant:abc", "tenant:xyz"})
}

func randomEventGen() *rapid.Generator[Event] {
	return rapid.Custom(func(t *rapid.T) Event {
		return Event{
			Type: RandomEventType(t),
			Tags: rapid.SliceOfNDistinct(randomEventTagGen(), 1, 3, func(i string) string { return i }).Draw(t, "tags"),
			Data: nillifyEmptySlice(rapid.SliceOfN(rapid.Byte(), 0, 1000).Draw(t, "data")),
		}
	})
}

func randomEventsGen() *rapid.Generator[[]Event] {
	return rapid.SliceOfN(randomEventGen(), 2, -1)
}

// this function is useful when comparing inserted values and the ones returned by fdb (it returns nil in case of empty array)
func nillifyEmptySlice[T any](s []T) []T {
	if s != nil && len(s) == 0 {
		return nil
	}
	return s
}

func CollectEvents(t *testing.T, seq func(func(StoredEvent, error) bool)) []StoredEvent {
	t.Helper()
	var events []StoredEvent
	for event, err := range seq {
		assert.NoError(t, err)
		events = append(events, event)
	}
	return events
}

func EventsAreStriclyOrdered(events []StoredEvent) bool {
	for i := 1; i < len(events); i++ {
		if events[i].Position.Compare(events[i-1].Position) <= 0 {
			return false
		}
	}
	return true
}
