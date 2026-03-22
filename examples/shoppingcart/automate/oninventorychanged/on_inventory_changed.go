package oninventorychanged

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/automate"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
)

func init() {
	Register(&automate.Registry)
}

func Register(registry *fairway.AutomationRegistry[automate.AllDeps]) {
	// This is an external event handler, registered as HTTP endpoint
	// The automation registry doesn't have HTTP support, so we'll use a different pattern
}

// RegisterHTTP registers the HTTP handler for external inventory events
func RegisterHTTP(mux *http.ServeMux, runner fairway.CommandRunner) {
	mux.HandleFunc("POST /webhooks/inventory-changed", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProductId string `json:"productId"`
			Inventory int    `json:"inventory"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := runner.RunPure(r.Context(), command{
			productId: req.ProductId,
			inventory: req.Inventory,
		}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}

type command struct {
	productId string
	inventory int
}

func (c command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	return ev.AppendEvents(ctx, fairway.NewEvent(event.InventoryChanged{
		ProductId: c.productId,
		Inventory: c.inventory,
	}))
}
