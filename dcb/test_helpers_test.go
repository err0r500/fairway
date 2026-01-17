//go:build test

package dcb

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventsAreStriclyOrdered_Ordered(t *testing.T) {
	// Given - strictly ordered events
	events := []StoredEvent{
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0}},
	}

	// When/Then
	assert.True(t, EventsAreStriclyOrdered(events))
}

func TestEventsAreStriclyOrdered_NotOrdered(t *testing.T) {
	// Given - events with same versionstamp (not strictly ordered)
	events := []StoredEvent{
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0}},
	}

	// When/Then - should return false because events[1] is not strictly greater than events[0]
	assert.False(t, EventsAreStriclyOrdered(events))
}

func TestEventsAreStriclyOrdered_Reversed(t *testing.T) {
	// Given - reversed events
	events := []StoredEvent{
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0}},
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}},
	}

	// When/Then
	assert.False(t, EventsAreStriclyOrdered(events))
}

func TestEventsAreStriclyOrdered_Empty(t *testing.T) {
	// Given - empty slice
	events := []StoredEvent{}

	// When/Then - empty is considered ordered
	assert.True(t, EventsAreStriclyOrdered(events))
}

func TestEventsAreStriclyOrdered_Single(t *testing.T) {
	// Given - single event
	events := []StoredEvent{
		{Position: Versionstamp{0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}},
	}

	// When/Then - single event is ordered
	assert.True(t, EventsAreStriclyOrdered(events))
}
