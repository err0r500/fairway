package fairway

import (
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// QueryItem represents a single event filter pattern.
// Types have OR semantics (match any), Tags have AND semantics (must have all).
type QueryItem struct {
	typeList     []string
	typeRegistry map[string]reflect.Type
	tagList      []string
}

// Types adds event types to match (OR semantics).
// Uses reflection to extract type names and store type info for deserialization.
func (q QueryItem) Types(events ...any) QueryItem {
	if q.typeRegistry == nil {
		q.typeRegistry = make(map[string]reflect.Type)
	}
	for _, e := range events {
		typeName := resolveEventTypeName(e)
		q.typeList = append(q.typeList, typeName)
		q.typeRegistry[typeName] = reflect.TypeOf(e)
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

// HandlerQuery represents the complete event filter for an event Handler
type HandlerQuery struct {
	Items []QueryItem
}

// convertQueryToDcb converts fairway.HandlerQuery to dcb.Query
func (q HandlerQuery) toDcb() *dcb.Query {
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
	query  HandlerQuery
	handle HandlerFunc
}

// Item creates a new QueryItem builder
func Item() QueryItem {
	return QueryItem{}
}

func Query(items ...QueryItem) HandlerQuery {
	return HandlerQuery{Items: items}
}

// Handle sets the handler function.
// Return false from the handler to stop iteration (break).
func (r HandlerQuery) Handle(fn HandlerFunc) *EventHandler {
	return &EventHandler{query: r, handle: fn}
}
