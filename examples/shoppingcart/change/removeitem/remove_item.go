package removeitem

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/change"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
)

func init() {
	Register(&change.ChangeRegistry)
}

func Register(registry *fairway.HttpChangeRegistry) {
	registry.RegisterCommand("DELETE /carts/{cartId}/items/{itemId}", httpHandler)
}

var errItemNotInCart = errors.New("the item doesn't belong to this cart")

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cartId := r.PathValue("cartId")
		itemId := r.PathValue("itemId")
		if cartId == "" || itemId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := runner.RunPure(r.Context(), command{
			cartId: cartId,
			itemId: itemId,
		}); err != nil {
			if errors.Is(err, errItemNotInCart) {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(err.Error())
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

type command struct {
	cartId string
	itemId string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	itemInCart := false
	alreadyRemoved := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.CartCreated{}, event.ItemAdded{}, event.ItemRemoved{}).
				Tags(event.CartIdTag(cmd.cartId)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.ItemAdded:
				if data.CartId == cmd.cartId && data.ItemId == cmd.itemId {
					itemInCart = true
				}
			case event.ItemRemoved:
				if data.CartId == cmd.cartId && data.ItemId == cmd.itemId {
					alreadyRemoved = true
				}
			}
			return true
		}); err != nil {
		return err
	}

	if alreadyRemoved {
		return nil // idempotent
	}

	if !itemInCart {
		return errItemNotInCart
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.ItemRemoved{
		CartId: cmd.cartId,
		ItemId: cmd.itemId,
	}))
}
