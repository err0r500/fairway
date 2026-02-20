package viewcartitems

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
	registry.RegisterView("GET /carts/{cartId}", httpHandler)
}

type CartItem struct {
	ItemId    string `json:"itemId"`
	ProductId string `json:"productId"`
	Price     int    `json:"price"`
	Quantity  int    `json:"quantity"`
}

type CartView struct {
	CartId     string     `json:"cartId"`
	Items      []CartItem `json:"items"`
	TotalPrice int        `json:"totalPrice"`
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cartId := r.PathValue("cartId")
		if cartId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		items := make(map[string]CartItem) // itemId -> CartItem
		cleared := false

		if err := reader.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.CartCreated{}, event.ItemAdded{}, event.ItemRemoved{}, event.CartCleared{}).
					Tags(event.CartIdTag(cartId)),
			),
			func(e fairway.Event) bool {
				switch data := e.Data.(type) {
				case event.ItemAdded:
					if data.CartId == cartId {
						items[data.ItemId] = CartItem{
							ItemId:    data.ItemId,
							ProductId: data.ProductId,
							Price:     data.Price,
							Quantity:  data.Quantity,
						}
					}
				case event.ItemRemoved:
					if data.CartId == cartId {
						delete(items, data.ItemId)
					}
				case event.CartCleared:
					if data.CartId == cartId {
						cleared = true
						items = make(map[string]CartItem)
					}
				}
				return true
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		_ = cleared // cleared tracked for potential future use

		var itemList []CartItem
		totalPrice := 0
		for _, item := range items {
			itemList = append(itemList, item)
			totalPrice += item.Price * item.Quantity
		}

		if itemList == nil {
			itemList = []CartItem{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(CartView{
			CartId:     cartId,
			Items:      itemList,
			TotalPrice: totalPrice,
		})
	}
}
