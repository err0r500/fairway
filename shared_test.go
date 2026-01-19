package fairway_test

import (
	"encoding/json"

	"github.com/err0r500/fairway/dcb"
	"pgregory.net/rapid"
)

// ============================================================================
// CONCRETE VALUES
// ============================================================================

var possibleTags = []string{
	"tag1", "tag2", "tag3", "tag-a", "tag-b", "tag-c",
}

// ============================================================================
// PUBLIC HELPERS (using generators)
// ============================================================================

func RandomEvent(t *rapid.T) any {
	return eventGen().Draw(t, "event")
}

func RandomEvents(t *rapid.T, maxCount int) []any {
	return eventsGen(maxCount).Draw(t, "events")
}

func RandomTags(t *rapid.T) []string {
	return tagsGen().Draw(t, "tags")
}

func RandomVersionstamp(t *rapid.T) dcb.Versionstamp {
	return versionstampGen().Draw(t, "versionstamp")
}

func RandomStoredEvent(t *rapid.T) dcb.StoredEvent {
	return storedEventGen().Draw(t, "storedEvent")
}

func RandomStoredEvents(t *rapid.T, maxCount int) []dcb.StoredEvent {
	return storedEventsGen(maxCount).Draw(t, "storedEvents")
}

func RandomStoredEventsMin(t *rapid.T, minCount, maxCount int) []dcb.StoredEvent {
	return rapid.SliceOfN(storedEventGen(), minCount, maxCount).Draw(t, "storedEvents")
}

// ============================================================================
// GENERATORS
// ============================================================================

func eventGen() *rapid.Generator[any] {
	return rapid.Custom(func(t *rapid.T) any {
		tags := RandomTags(t)
		eventType := rapid.IntRange(0, 2).Draw(t, "eventType")
		switch eventType {
		case 0:
			return TestEventA{Value: rapid.String().Draw(t, "value"), eventTags: eventTags{value: tags}}
		case 1:
			return TestEventB{Count: rapid.Int().Draw(t, "count"), eventTags: eventTags{value: tags}}
		default:
			return TestEventC{Flag: rapid.Bool().Draw(t, "flag"), eventTags: eventTags{value: tags}}
		}
	})
}

func eventsGen(maxCount int) *rapid.Generator[[]any] {
	return rapid.SliceOfN(eventGen(), 1, maxCount)
}

func tagsGen() *rapid.Generator[[]string] {
	return rapid.Custom(func(t *rapid.T) []string {
		numTags := rapid.IntRange(0, 5).Draw(t, "numTags")
		if numTags == 0 {
			return nil
		}
		tags := make([]string, numTags)
		for i := range numTags {
			tags[i] = rapid.SampledFrom(possibleTags).Draw(t, "tag")
		}
		return tags
	})
}

func versionstampGen() *rapid.Generator[dcb.Versionstamp] {
	return rapid.Custom(func(t *rapid.T) dcb.Versionstamp {
		var vs dcb.Versionstamp
		bytes := rapid.SliceOfN(rapid.Byte(), 12, 12).Draw(t, "vsBytes")
		copy(vs[:], bytes)
		return vs
	})
}

func storedEventGen() *rapid.Generator[dcb.StoredEvent] {
	return rapid.Custom(func(t *rapid.T) dcb.StoredEvent {
		eventType := rapid.IntRange(0, 2).Draw(t, "eventType")

		var event any
		var typeName string

		switch eventType {
		case 0:
			event = TestEventA{Value: rapid.String().Draw(t, "value")}
			typeName = "TestEventA"
		case 1:
			event = TestEventB{Count: rapid.Int().Draw(t, "count")}
			typeName = "TestEventB"
		default:
			event = TestEventC{Flag: rapid.Bool().Draw(t, "flag")}
			typeName = "TestEventC"
		}

		// Serialize the event to JSON (like the real system does)
		data, _ := json.Marshal(event)

		return dcb.StoredEvent{
			Event: dcb.Event{
				Type: typeName,
				Tags: RandomTags(t),
				Data: data,
			},
			Position: RandomVersionstamp(t),
		}
	})
}

func storedEventsGen(maxCount int) *rapid.Generator[[]dcb.StoredEvent] {
	return rapid.SliceOfN(storedEventGen(), 1, maxCount)
}
