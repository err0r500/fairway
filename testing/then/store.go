package then

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
)

type eventForComparison struct {
	Type string
	Tags []string
	Data json.RawMessage
}

func ExpectEventsInStore(t *testing.T, store dcb.DcbStore, events ...fairway.Event) {
	ctx := context.Background()

	var expectedEvents []eventForComparison
	for _, ev := range events {
		dcbEvent, err := fairway.ToDcbEvent(ev)
		assert.NoError(t, err)
		expectedEvents = append(expectedEvents, eventForComparison{
			Type: dcbEvent.Type,
			Tags: dcbEvent.Tags,
			Data: extractData(dcbEvent.Data),
		})
	}

	var eventsInStore []eventForComparison
	for e, err := range store.ReadAll(ctx) {
		assert.NoError(t, err)
		eventsInStore = append(eventsInStore, eventForComparison{
			Type: e.Type,
			Tags: e.Tags,
			Data: extractData(e.Data),
		})
	}

	assert.ElementsMatch(t, expectedEvents, eventsInStore)
}

func extractData(raw []byte) json.RawMessage {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	return envelope.Data
}
