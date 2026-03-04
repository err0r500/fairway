package opencartswithproducts

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
	"github.com/err0r500/fairway/examples/shoppingcart/view"
	"github.com/err0r500/fairway/utils"
)

var readModel *fairway.ReadModel[CartItem, Repo]

func init() {
	view.ReadModelRegistry.Register(func(store dcb.DcbStore) (fairway.ReadModelStarter, error) {
		rm, err := NewReadModel(store)
		if err != nil {
			return nil, err
		}
		readModel = rm
		return rm, nil
	})
}

// Register registers the HTTP handler (call after StartAll)
// func Register(mux *http.ServeMux) {
// 	mux.HandleFunc("GET /open-carts/{productId}", HTTPHandler(readModel))
// }

type CartItem struct {
	CartId string `json:"cartId"`
	ItemId string `json:"itemId"`
}

type OpenCartsView struct {
	Carts []CartItem `json:"carts"`
}

// Repo provides data operations for the read model
type Repo struct {
	kv utils.KV
}

func (r Repo) AddItem(productId, cartId, itemId string) {
	r.kv.SetPath([]string{productId, cartId, itemId})
	r.kv.SetPath([]string{"r", cartId, itemId, productId})
}

func (r Repo) ClearItem(cartId, itemId string) {
	keys := r.kv.ScanPath([]string{"r", cartId, itemId})
	for _, key := range keys {
		if len(key) >= 4 {
			productId := key[3]
			r.kv.ClearPath([]string{productId, cartId, itemId})
			r.kv.ClearPath([]string{"r", cartId, itemId, productId})
		}
	}
}

func (r Repo) ClearCart(cartId string) {
	keys := r.kv.ScanPath([]string{"r", cartId})
	for _, key := range keys {
		if len(key) >= 4 {
			itemId, productId := key[2], key[3]
			r.kv.ClearPath([]string{productId, cartId, itemId})
		}
	}
	r.kv.ClearPrefix([]string{"r", cartId})
}

// HTTPHandler returns an HTTP handler that queries the read model
func HTTPHandler(rm *fairway.ReadModel[CartItem, Repo]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rm.IsCaughtUp() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		productId := r.PathValue("productId")
		if productId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		keys, err := rm.Scan(r.Context(), fairway.P(productId))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		carts := make([]CartItem, 0, len(keys))
		for _, key := range keys {
			if len(key) >= 3 {
				carts = append(carts, CartItem{CartId: key[1], ItemId: key[2]})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(OpenCartsView{Carts: carts})
	}
}

// NewReadModel creates the persistent read model for open carts with products
// Key structure:
//   - P(productId, cartId, itemId) -> nil (primary, for queries)
//   - P("r", cartId, itemId) -> productId (reverse, for clearing)
func NewReadModel(store dcb.DcbStore) (*fairway.ReadModel[CartItem, Repo], error) {
	return fairway.NewReadModel(
		store,
		"open-carts-with-products",
		[]any{
			event.ItemAdded{},
			event.ItemRemoved{},
			event.CartCleared{},
			event.CartSubmitted{},
			event.ItemArchived{},
		},
		func(tr fdb.Transaction, space subspace.Subspace) Repo {
			return Repo{kv: utils.NewKV(tr, space)}
		},
		handler,
		fairway.WithReadModelPollInterval[CartItem, Repo](50*time.Millisecond),
	)
}

func handler(repo Repo, ev fairway.Event) error {
	switch data := ev.Data.(type) {
	case event.ItemAdded:
		repo.AddItem(data.ProductId, data.CartId, data.ItemId)
	case event.ItemRemoved:
		repo.ClearItem(data.CartId, data.ItemId)
	case event.ItemArchived:
		repo.ClearItem(data.CartId, data.ItemId)
	case event.CartCleared:
		repo.ClearCart(data.CartId)
	case event.CartSubmitted:
		repo.ClearCart(data.CartId)
	}
	return nil
}
