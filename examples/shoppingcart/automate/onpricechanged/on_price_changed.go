package onpricechanged

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
)

// RegisterHTTP registers the HTTP handler for external price change events
func RegisterHTTP(mux *http.ServeMux, runner fairway.CommandRunner) {
	mux.HandleFunc("POST /webhooks/price-changed", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ProductId string `json:"productId"`
			OldPrice  int    `json:"oldPrice"`
			NewPrice  int    `json:"newPrice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if err := runner.RunPure(r.Context(), command{
			productId: req.ProductId,
			oldPrice:  req.OldPrice,
			newPrice:  req.NewPrice,
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
	oldPrice  int
	newPrice  int
}

func (c command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	return ev.AppendEvents(ctx, fairway.NewEvent(event.PriceChanged{
		ProductId: c.productId,
		OldPrice:  c.oldPrice,
		NewPrice:  c.newPrice,
	}))
}
