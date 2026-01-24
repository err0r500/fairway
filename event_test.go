package fairway

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewEvent_SetsTimestamp(t *testing.T) {
	before := time.Now()
	e := NewEvent(struct{}{})
	after := time.Now()

	assert.True(t, e.OccuredAt().After(before))
	assert.True(t, e.OccuredAt().Before(after))
}

func TestNewEventAt_SetsExplicitTimestamp(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	e := NewEventAt(struct{}{}, ts)

	assert.Equal(t, ts, e.OccuredAt())
}

func TestOccuredAt_ReturnsTimestamp(t *testing.T) {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	e := Event{OccurredAt: ts, Data: nil}

	assert.Equal(t, ts, e.OccuredAt())
}
