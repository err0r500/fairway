package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// eventRegistry maps event type names to their Go types for deserialization
type eventRegistry struct {
	types map[string]reflect.Type
}

// newEventRegistry creates a new event registry
func newEventRegistry() eventRegistry {
	return eventRegistry{types: make(map[string]reflect.Type)}
}

// register registers an event type for deserialization
func (r *eventRegistry) register(events ...any) {
	for _, e := range events {
		r.types[resolveEventTypeName(e)] = reflect.TypeOf(e)
	}
}

// deserialize converts dcb.Event to typed event
func (r eventRegistry) deserialize(de dcb.Event) (any, error) {
	typ, ok := r.types[de.Type]
	if !ok {
		return nil, fmt.Errorf("unknown event type: %s", de.Type)
	}

	// Create new instance
	ptr := reflect.New(typ)

	// Unmarshal JSON data into it
	if err := json.Unmarshal(de.Data, ptr.Interface()); err != nil {
		return nil, fmt.Errorf("failed to deserialize %s: %w", de.Type, err)
	}

	return ptr.Elem().Interface(), nil
}
