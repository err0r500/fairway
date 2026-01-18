package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// Tagger is an interface for events that can provide tags for indexing.
// Tags are derived from the event's internal data and used for querying,
// but are not stored as part of the event payload.
type Tagger interface {
	Tags() []string
}

// Typer is anything that can provide an event type string
type Typer interface {
	TypeString() string
}

// resolveEventTypeName determines the event type name for an event instance.
//
//  1. If the event implements Typer interface, use EventType() method
//  2. Otherwise, fall back to the struct's type name via reflection
func resolveEventTypeName(event any) string {
	if typer, ok := event.(Typer); ok {
		return typer.TypeString()
	}

	return reflect.TypeOf(event).Name()
}

// extractTags extracts tags from an event if it implements the Tagger interface
func extractTags(event any) []string {
	if tagger, ok := event.(Tagger); ok {
		return tagger.Tags()
	}
	return nil
}

// ToDcbEvent serializes events using JSON and extracts tags for indexing
func ToDcbEvent(event any) (dcb.Event, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return dcb.Event{}, fmt.Errorf("failed to serialize event: %w", err)
	}

	return dcb.Event{
		Type: resolveEventTypeName(event),
		Data: data,
		Tags: extractTags(event),
	}, nil
}
