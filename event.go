package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// Event is the required interface for all events.
// Tags are derived from the event's internal data and used for querying/indexing,
// but are not stored as part of the event payload.
type Event interface {
	Tags() []string
}

// Typer is an optional interface for events that want to override their type name.
// If not implemented, the type name is derived from reflection.
type Typer interface {
	TypeString() string
}

// resolveEventTypeName determines the event type name for an event instance.
//
//  1. If the event implements Typer interface, use TypeString() method
//  2. Otherwise, fall back to the struct's type name via reflection
func resolveEventTypeName(event Event) string {
	if typer, ok := event.(Typer); ok {
		return typer.TypeString()
	}

	return reflect.TypeOf(event).Name()
}

// ToDcbEvent serializes events using JSON and extracts tags for indexing
func ToDcbEvent(event Event) (dcb.Event, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return dcb.Event{}, fmt.Errorf("failed to serialize event: %w", err)
	}

	return dcb.Event{
		Type: resolveEventTypeName(event),
		Data: data,
		Tags: event.Tags(),
	}, nil
}
