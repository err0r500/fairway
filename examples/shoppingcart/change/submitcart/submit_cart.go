package submitcart

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
	registry.RegisterCommand("POST /cart/{cartId}/submit", httpHandler)
}

var (
	errCartNotFound      = errors.New("cart not found")
	errCartEmpty         = errors.New("cart is empty")
	errCartAlreadyClosed = errors.New("cart already submitted or closed")
)

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cartId := r.PathValue("cartId")
		if cartId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := runner.RunPure(r.Context(), command{cartId: cartId}); err != nil {
			switch {
			case errors.Is(err, errCartNotFound):
				w.WriteHeader(http.StatusNotFound)
			case errors.Is(err, errCartEmpty):
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(err.Error())
			case errors.Is(err, errCartAlreadyClosed):
				w.WriteHeader(http.StatusConflict)
			default:
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err.Error())
			}
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
	itemCount := 0
	cartClosed := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.CartCreated{}, event.ItemAdded{}, event.ItemRemoved{}, event.CartCleared{}, event.CartSubmitted{}, event.CartClosed{}).
				Tags(event.CartIdTag(cmd.cartId)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.CartCreated:
				cartExists = true
			case event.ItemAdded:
				itemCount++
			case event.ItemRemoved:
				itemCount--
			case event.CartCleared:
				itemCount = 0
			case event.CartSubmitted:
				if data.CartId == cmd.cartId {
					cartClosed = true
				}
			case event.CartClosed:
				if data.CartId == cmd.cartId {
					cartClosed = true
				}
			}
			return true
		}); err != nil {
		return err
	}

	if !cartExists {
		return errCartNotFound
	}

	if cartClosed {
		return errCartAlreadyClosed
	}

	if itemCount <= 0 {
		return errCartEmpty
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.CartSubmitted{CartId: cmd.cartId}))
}
