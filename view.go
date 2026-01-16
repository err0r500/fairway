package fairway

import "context"

type EventsReader interface {
	ReadEvents(ctx context.Context, router *EventHandler) error
}
