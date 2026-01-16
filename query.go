package fairway

import "github.com/err0r500/fairway/dcb"

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
