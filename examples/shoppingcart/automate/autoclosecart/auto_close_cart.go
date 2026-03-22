package autoclosecart

import (
	"context"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/shoppingcart/automate"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
)

func init() {
	Register(&automate.Registry)
}

type command struct {
	CartId string
}

func Register(registry *fairway.AutomationRegistry[automate.AllDeps]) {
	registry.RegisterAutomation(
		func(store dcb.DcbStore, deps automate.AllDeps) (fairway.Startable, error) {
			return fairway.NewAutomation(
				store,
				struct{}{},
				"auto-close-cart",
				event.CartSubmitted{},
				eventToCommand,
			)
		},
	)
}

func eventToCommand(ev fairway.Event) fairway.CommandWithEffect[struct{}] {
	data := ev.Data.(event.CartSubmitted)
	return command{CartId: data.CartId}
}

func (c command) Run(ctx context.Context, ra fairway.EventReadAppenderExtended, _ struct{}) error {
	alreadyClosed := false

	if err := ra.ReadEvents(ctx, fairway.QueryItems(
		fairway.NewQueryItem().Types(event.CartClosed{}).Tags(event.CartIdTag(c.CartId)),
	), func(e fairway.Event) bool {
		alreadyClosed = true
		return false
	}); err != nil {
		return err
	}

	if alreadyClosed {
		return nil
	}

	return ra.AppendEventsNoCondition(ctx, fairway.NewEvent(event.CartClosed{CartId: c.CartId}))
}
