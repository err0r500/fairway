package fairway

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

type EventsReader interface {
	ReadEvents(ctx context.Context, eventHandler *EventHandler) error
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
func (ra viewReader) ReadEvents(ctx context.Context, eventHandler *EventHandler) error {
	if eventHandler.handle == nil {
		return nil
	}

	// Auto-register types from query
	for _, item := range eventHandler.query.items {
		ra.eventRegistry.registerTypes(item.typeRegistry)
	}

	for dcbStoredEvent, err := range ra.store.Read(ctx, *eventHandler.query.toDcb(), nil) {
		if err != nil {
			return err
		}

		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Deserialize dcb.Event → event
		fairwayEvent, err := ra.eventRegistry.deserialize(dcbStoredEvent.Event)
		if err != nil {
			return err
		}

		// Dispatch TaggedEvent to handler
		if !eventHandler.handle(TaggedEvent{Event: fairwayEvent, Tags: dcbStoredEvent.Tags}, nil) {
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
	for typeName, typ := range types {
		r.types[typeName] = typ
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


