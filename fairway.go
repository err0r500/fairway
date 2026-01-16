package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// Typer is anything that can provide an event type string
type Typer interface {
	EventType() string
}

// TaggedEvent wraps an event with optional tags
type TaggedEvent struct {
	Event any      // the actual event struct
	Tags  []string // optional tags for categorization
}

// NewEvent creates a TaggedEvent with tags
func NewEvent(event any, tags ...string) TaggedEvent {
	return TaggedEvent{Event: event, Tags: tags}
}

// resolveEventTypeName determines the event type name for an event instance.
//
// Resolution Priority:
//  1. If the event implements Typer interface, use EventType() method
//  2. If the event struct has an "EventType" string field with a non-empty value, use it
//  3. Otherwise, fall back to the struct's type name via reflection
func resolveEventTypeName(event any) string {
	// Priority 1: Check if implements Typer
	if typer, ok := event.(Typer); ok {
		return typer.EventType()
	}

	// Priority 2: Check for EventType field
	v := reflect.ValueOf(event)
	if v.Kind() == reflect.Struct {
		field := v.FieldByName("EventType")
		if field.IsValid() && field.Kind() == reflect.String {
			if eventType := field.String(); eventType != "" {
				return eventType
			}
		}
	}

	// Priority 3: Fall back to type name
	return reflect.TypeOf(event).Name()
}

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

// --- Query Types ---

// QueryItem represents a single event filter pattern.
// Types have OR semantics (match any), Tags have AND semantics (must have all).
type QueryItem struct {
	typeList       []string
	tagList        []string
	eventInstances []any
}

// Types adds event types to match (OR semantics).
// Uses reflection to extract type names from event instances.
func (q QueryItem) Types(events ...any) QueryItem {
	for _, e := range events {
		q.typeList = append(q.typeList, resolveEventTypeName(e))
		q.eventInstances = append(q.eventInstances, e)
	}
	return q
}

// Tags adds required tags (AND semantics)
func (q QueryItem) Tags(tags ...string) QueryItem {
	q.tagList = append(q.tagList, tags...)
	return q
}

// toDcb converts to dcb.QueryItem
func (q QueryItem) toDcb() dcb.QueryItem {
	return dcb.QueryItem{
		Types: q.typeList,
		Tags:  q.tagList,
	}
}

// RouterQuery represents the complete event filter for a router
type RouterQuery struct {
	Items []QueryItem
}

// convertQueryToDcb converts fairway.RouterQuery to dcb.Query
func (q RouterQuery) toDcb() *dcb.Query {
	items := make([]dcb.QueryItem, len(q.Items))
	for i, item := range q.Items {
		items[i] = item.toDcb()
	}
	return &dcb.Query{Items: items}
}

// HandlerFunc processes an event. Return false to stop iteration.
type HandlerFunc func(TaggedEvent, error) bool

// EventHandler routes events from an EventStore to a handler.
type EventHandler struct {
	query  RouterQuery
	handle HandlerFunc
}

// Query creates a new Router with the specified query items
func Query(items ...QueryItem) RouterQuery {
	return RouterQuery{Items: items}
}

// Handle sets the handler function.
// Return false from the handler to stop iteration (break).
func (r RouterQuery) Handle(fn HandlerFunc) *EventHandler {
	return &EventHandler{query: r, handle: fn}
}
