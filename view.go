package fairway

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"time"

	"github.com/err0r500/fairway/dcb"
)

type EventsReader interface {
	ReadEvents(ctx context.Context, query Query, handler HandlerFunc) error
}

// commandReadAppender provides read-then-conditional-append for commands
type viewReader struct {
	store         dcb.DcbStore
	eventRegistry eventRegistry
}

// NewReader creates a Events with given store
func NewReader(store dcb.DcbStore) EventsReader {
	return viewReader{
		store:         store,
		eventRegistry: newEventRegistry(),
	}
}

// ReadEvents reads events using the eventHandler's query and dispatches to handlers
func (ra viewReader) ReadEvents(ctx context.Context, query Query, handler HandlerFunc) error {
	if handler == nil {
		return nil
	}

	// Auto-register types from query
	for _, item := range query.items {
		ra.eventRegistry.registerTypes(item.typeRegistry)
	}

	for dcbStoredEvent, err := range ra.store.Read(ctx, *query.toDcb(), nil) {
		if err != nil {
			// context errors already have context
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("reading events: %s", err)
		}

		// Deserialize dcb.Event → Event
		ev, err := ra.eventRegistry.deserialize(dcbStoredEvent.Event)
		if err != nil {
			return fmt.Errorf("deserializing event at position %x: %s", dcbStoredEvent.Position[:], err)
		}

		// Dispatch Event to handler
		if !handler(ev) {
			return nil
		}
	}

	return nil
}

// eventRegistry maps event type names to their Go types for deserialization
type eventRegistry struct {
	types map[string]reflect.Type
}

// newEventRegistry creates a new event registry
func newEventRegistry() eventRegistry {
	return eventRegistry{types: make(map[string]reflect.Type)}
}

// registerTypes registers event types from a type registry map
func (r *eventRegistry) registerTypes(types map[string]reflect.Type) {
	maps.Copy(r.types, types)
}

// registeredTypeNames returns list of registered type names for error context
func (r eventRegistry) registeredTypeNames() []string {
	names := make([]string, 0, len(r.types))
	for name := range r.types {
		names = append(names, name)
	}
	return names
}

// deserialize converts dcb.Event to Event
func (r eventRegistry) deserialize(de dcb.Event) (Event, error) {
	typ, ok := r.types[de.Type]
	if !ok {
		return Event{}, fmt.Errorf("unknown event type %q (registered: %v)", de.Type, r.registeredTypeNames())
	}

	// Unmarshal envelope to get timestamp and raw data
	var envelope struct {
		OccurredAt time.Time       `json:"occurredAt"`
		Data       json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(de.Data, &envelope); err != nil {
		return Event{}, fmt.Errorf("json unmarshal envelope for event type %q: %s", de.Type, err)
	}

	// Create new instance of user's data type
	ptr := reflect.New(typ)

	// Unmarshal inner data into it
	if err := json.Unmarshal(envelope.Data, ptr.Interface()); err != nil {
		return Event{}, fmt.Errorf("json unmarshal data for event type %q: %s", de.Type, err)
	}

	return Event{
		OccurredAt: envelope.OccurredAt,
		Data:       ptr.Elem().Interface(),
	}, nil
}
