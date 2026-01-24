package fairway

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/err0r500/fairway/dcb"
)

// Event wraps user's data with timestamp - the main event type
type Event struct {
	OccurredAt time.Time `json:"occurredAt"`
	Data       any       `json:"data"`
}

// NewEvent creates an event with auto-generated timestamp
func NewEvent(data any) Event {
	return Event{OccurredAt: time.Now(), Data: data}
}

// NewEventAt creates an event with explicit timestamp (for migrations, replays, tests)
func NewEventAt(data any, ts time.Time) Event {
	return Event{OccurredAt: ts, Data: data}
}

// OccuredAt returns the event's occurrence time
func (e Event) OccuredAt() time.Time { return e.OccurredAt }

// Tags returns tags from the underlying data if it implements Tags() []string
func (e Event) Tags() []string {
	if tagger, ok := e.Data.(interface{ Tags() []string }); ok {
		return tagger.Tags()
	}
	return []string{}
}

// typeString returns the type name for registry lookup
func (e Event) typeString() string {
	if typer, ok := e.Data.(interface{ TypeString() string }); ok {
		return typer.TypeString()
	}
	return reflect.TypeOf(e.Data).Name()
}

// ToDcbEvent serializes an Event to dcb.Event
func ToDcbEvent(e Event) (dcb.Event, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return dcb.Event{}, fmt.Errorf("failed to serialize event: %w", err)
	}

	return dcb.Event{
		Type: e.typeString(),
		Data: data,
		Tags: e.Tags(),
	}, nil
}
