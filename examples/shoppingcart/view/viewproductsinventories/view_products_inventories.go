package viewproductsinventories

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
	registry.RegisterView("GET /inventories/products/{productId}", httpHandler)
}

type ProductInventory struct {
	ProductId string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

type InventoriesView struct {
	Products []ProductInventory `json:"products"`
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		productId := r.PathValue("productId")
		if productId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var inventory *int

		if err := reader.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.InventoryChanged{}).
					Tags(event.ProductIdTag(productId)),
			),
			func(e fairway.Event) bool {
				if data, ok := e.Data.(event.InventoryChanged); ok {
					inventory = &data.Inventory
				}
				return true // get latest
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		products := []ProductInventory{}
		if inventory != nil {
			products = append(products, ProductInventory{
				ProductId: productId,
				Quantity:  *inventory,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(InventoriesView{Products: products})
	}
}
