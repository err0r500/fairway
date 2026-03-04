package additem

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/shoppingcart/change"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
	"github.com/err0r500/fairway/utils"
)

func init() {
	Register(&change.ChangeRegistry)
}

func Register(registry *fairway.HttpChangeRegistry) {
	registry.RegisterCommand("POST /carts/{cartId}/items", httpHandler)
}

var (
	errAlreadyCreated = errors.New("already created")
	errMaxItems       = errors.New("can't add more than 3 items")
	errNoInventory    = errors.New("product out of stock")
)

type reqBody struct {
	ItemId      string `json:"itemId" validate:"required"`
	ProductId   string `json:"productId" validate:"required"`
	Description string `json:"description"`
	ImageURL    string `json:"imageURL"`
	Price       int    `json:"price" validate:"required"`
	Quantity    int    `json:"quantity"`
}

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cartId := r.PathValue("cartId")
		if cartId == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var req reqBody
		if err := utils.JsonParse(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		quantity := req.Quantity
		if quantity == 0 {
			quantity = 1
		}

		if err := runner.RunPure(r.Context(), command{
			cartId:      cartId,
			itemId:      req.ItemId,
			productId:   req.ProductId,
			description: req.Description,
			image:       req.ImageURL,
			price:       req.Price,
			quantity:    quantity,
		}); err != nil {
			switch {
			case errors.Is(err, errAlreadyCreated):
				w.WriteHeader(http.StatusConflict)
			case errors.Is(err, errMaxItems):
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(err.Error())
			case errors.Is(err, errNoInventory):
				w.WriteHeader(http.StatusUnprocessableEntity)
				json.NewEncoder(w).Encode(err.Error())
			default:
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err.Error())
			}
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

type command struct {
	cartId      string
	itemId      string
	productId   string
	description string
	image       string
	price       int
	quantity    int
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	cartExists := false
	itemCount := 0
	inventory := -1 // -1 means no inventory info

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.CartCreated{}, event.ItemAdded{}).
				Tags(event.CartIdTag(cmd.cartId)),
			fairway.NewQueryItem().
				Types(event.InventoryChanged{}).
				Tags(event.ProductIdTag(cmd.productId)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.CartCreated:
				if data.CartId == cmd.cartId {
					cartExists = true
				}
			case event.ItemAdded:
				if data.CartId == cmd.cartId {
					itemCount++
				}
			case event.InventoryChanged:
				if data.ProductId == cmd.productId {
					inventory = data.Inventory
				}
			}
			return true
		}); err != nil {
		return err
	}

	// Check inventory
	if inventory == 0 {
		return errNoInventory
	}

	// Check max items
	if itemCount >= 3 {
		return errMaxItems
	}

	itemAdded := fairway.NewEvent(event.ItemAdded{
		CartId:      cmd.cartId,
		ItemId:      cmd.itemId,
		ProductId:   cmd.productId,
		Description: cmd.description,
		Image:       cmd.image,
		Price:       cmd.price,
		Quantity:    cmd.quantity,
	})

	if !cartExists {
		return ev.AppendEvents(ctx, fairway.NewEvent(event.CartCreated{CartId: cmd.cartId}), itemAdded)
	}

	return ev.AppendEvents(ctx, itemAdded)
}
