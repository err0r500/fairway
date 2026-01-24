package fairway

import (
	"reflect"

	"github.com/err0r500/fairway/dcb"
)

// Query represents the complete event filter for an event Handler
type Query struct {
	items []QueryItem
}

// QueryItem represents a single event filter pattern.
// Types have OR semantics (match any), Tags have AND semantics (must have all).
type QueryItem struct {
	typeList     []string                // used for building dbc.Query
	tagList      []string                // used for building dbc.Query
	typeRegistry map[string]reflect.Type // used for deserialization of events based on their type
}

// HandlerFunc processes an event. Return false to stop iteration.
type HandlerFunc func(Event) bool

// resolveEventTypeName determines the event type name for an event instance.
func resolveEventTypeName(event any) string {
	if typer, ok := event.(interface{ TypeString() string }); ok {
		return typer.TypeString()
	}
	return reflect.TypeOf(event).Name()
}

// convertQueryToDcb converts fairway.HandlerQuery to dcb.Query
func (q Query) toDcb() *dcb.Query {
	items := make([]dcb.QueryItem, len(q.items))
	for i, item := range q.items {
		items[i] = item.toDcb()
	}
	return &dcb.Query{Items: items}
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

// NewQueryItem creates a new QueryItem builder
func NewQueryItem() QueryItem {
	return QueryItem{}
}

func QueryItems(items ...QueryItem) Query {
	return Query{items: items}
}
