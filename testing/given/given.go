package given

import (
	"context"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
)

// EventsInStore appends events to store without condition (for test setup)
func EventsInStore(store dcb.DcbStore, e any, ee ...any) error {
	ctx := context.Background()

	allEvents := append([]any{e}, ee...)
	dcbEvents := make([]dcb.Event, len(allEvents))

	for i, event := range allEvents {
		dcbEvent, err := fairway.ToDcbEvent(event)
		if err != nil {
			return err
		}
		dcbEvents[i] = dcbEvent
	}

	return store.Append(ctx, dcbEvents, nil)
}
