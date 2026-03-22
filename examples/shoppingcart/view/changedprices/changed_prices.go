package changedprices

import (
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
	"github.com/err0r500/fairway/examples/shoppingcart/view"
)

func init() {
	Register(&view.ViewRegistry)
}

func Register(registry *fairway.HttpViewRegistry) {
	registry.RegisterView("GET /prices/products/{productId}", httpHandler)
}

type ProductPrice struct {
	ProductId string `json:"productId"`
	OldPrice  int    `json:"oldPrice"`
	NewPrice  int    `json:"newPrice"`
}

type PricesView struct {
	Products []ProductPrice `json:"products"`
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productId := r.PathValue("productId")
		if productId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var latestPrice *ProductPrice

		if err := reader.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.PriceChanged{}).
					Tags(event.ProductIdTag(productId)),
			),
			func(e fairway.Event) bool {
				if data, ok := e.Data.(event.PriceChanged); ok {
					latestPrice = &ProductPrice{
						ProductId: data.ProductId,
						OldPrice:  data.OldPrice,
						NewPrice:  data.NewPrice,
					}
				}
				return true // get latest
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		products := []ProductPrice{}
		if latestPrice != nil {
			products = append(products, *latestPrice)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(PricesView{Products: products})
	}
}
