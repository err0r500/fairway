package archiveitems

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
	ProductId string
	deps      automate.AllDeps
}

func Register(registry *fairway.AutomationRegistry[automate.AllDeps]) {
	registry.RegisterAutomation(
		func(store dcb.DcbStore, deps automate.AllDeps) (fairway.Startable, error) {
			return fairway.NewAutomation(
				store,
				deps,
				"archive-items",
				event.PriceChanged{},
				eventToCommand,
			)
		},
	)
}

func eventToCommand(ev fairway.Event) fairway.CommandWithEffect[automate.AllDeps] {
	data := ev.Data.(event.PriceChanged)
	return command{ProductId: data.ProductId}
}

func (c command) Run(ctx context.Context, ra fairway.EventReadAppenderExtended, deps automate.AllDeps) error {
	// Get all cart items with this product from the read model
	keys, err := deps.OpenCartsRM.Scan(ctx, fairway.P(c.ProductId))
	if err != nil {
		return err
	}

	// Archive each item
	for _, key := range keys {
		if len(key) < 3 {
			continue
		}
		cartId, itemId := key[1], key[2]
		if err := ra.AppendEventsNoCondition(ctx, fairway.NewEvent(event.ItemArchived{
			CartId: cartId,
			ItemId: itemId,
		})); err != nil {
			return err
		}
	}

	return nil
}
