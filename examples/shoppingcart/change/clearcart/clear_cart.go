package clearcart

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/change"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
)

func init() {
	Register(&change.ChangeRegistry)
}

func Register(registry *fairway.HttpChangeRegistry) {
	registry.RegisterCommand("DELETE /carts/{cartId}/items", httpHandler)
}

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cartId := r.PathValue("cartId")
		if cartId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := runner.RunPure(r.Context(), command{cartId: cartId}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

type command struct {
	cartId string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	cartExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.CartCreated{}).
				Tags(event.CartIdTag(cmd.cartId)),
		),
		func(e fairway.Event) bool {
			if _, ok := e.Data.(event.CartCreated); ok {
				cartExists = true
				return false
			}
			return true
		}); err != nil {
		return err
	}

	if !cartExists {
		return nil // nothing to clear
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.CartCleared{CartId: cmd.cartId}))
}
