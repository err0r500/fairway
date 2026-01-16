package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// TaggedEvent wraps an event with optional tags
type TaggedEvent struct {
	Event any      // the actual event struct
	Tags  []string // optional tags for categorization
}

// NewEvent creates a TaggedEvent with tags
func NewEvent(event any, tags ...string) TaggedEvent {
	return TaggedEvent{Event: event, Tags: tags}
}

// Typer is anything that can provide an event type string
type Typer interface {
	EventType() string
}

// resolveEventTypeName determines the event type name for an event instance.
//
//  1. If the event implements Typer interface, use EventType() method
//  2. Otherwise, fall back to the struct's type name via reflection
func resolveEventTypeName(event any) string {
	if typer, ok := event.(Typer); ok {
		return typer.EventType()
	}

	return reflect.TypeOf(event).Name()
}

// toDcbEvent serializes events using JSON
func toDcbEvent(e TaggedEvent) (dcb.Event, error) {
	data, err := json.Marshal(e.Event)
	if err != nil {
		return dcb.Event{}, fmt.Errorf("failed to serialize event: %w", err)
	}

	return dcb.Event{
		Type: resolveEventTypeName(e.Event),
		Data: data,
		Tags: e.Tags,
	}, nil
}
