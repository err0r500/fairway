package then

import (
	"context"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
)

func ExpectEventsInStore(t *testing.T, store dcb.DcbStore, events ...fairway.TaggedEvent) {
	ctx := context.Background()

	var expectedEvents []dcb.Event
	for _, taggedEvt := range events {
		dcbEvent, err := fairway.ToDcbEvent(taggedEvt)
		assert.NoError(t, err)
		expectedEvents = append(expectedEvents, dcbEvent)
	}

	var eventsInStore []dcb.Event
	for e, err := range store.ReadAll(ctx) {
		assert.NoError(t, err)
		eventsInStore = append(eventsInStore, dcb.Event{Type: e.Type, Data: e.Data, Tags: e.Tags})
	}

	assert.ElementsMatch(t, expectedEvents, eventsInStore)
}
