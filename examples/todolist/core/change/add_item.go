package change

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/core/event"
)

func init() {
	ChangeRegistry.RegisterCommand("POST /api/lists/{listId}/items/{itemId}", addItemHttpHandler)
}

var itemAlreadyExistsErr = errors.New("item already exists")

type addItemHttpReq struct {
	Text string `json:"text" validate:"required"`
}

func addItemHttpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req addItemHttpReq
		if !fairway.JsonParse(w, r, &req) {
			return
		}

		if err := runner.RunPure(r.Context(), addItem{
			listId: r.PathValue("listId"),
			itemId: r.PathValue("itemId"),
			text:   req.Text,
		}); err != nil {
			switch err {
			case itemAlreadyExistsErr:
				w.WriteHeader(http.StatusConflict)

			default:
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			}
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

type addItem struct {
	listId string
	itemId string
	text   string
}

func (cmd addItem) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	itemAlreadyExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().Types(event.ListItemAdded{}).Tags("item_id:"+cmd.itemId),
		),
		func(te fairway.TaggedEvent, _ error) bool {
			switch te.Event.(type) {
			case event.ListItemAdded:
				itemAlreadyExists = true
				return false
			default:
				return true
			}
		}); err != nil {
		return err
	}

	if itemAlreadyExists {
		return itemAlreadyExistsErr
	}

	return ev.AppendEvents(ctx,
		fairway.Event(
			event.ListItemAdded{ListId: cmd.listId, ItemId: cmd.itemId, Text: cmd.text},
			"list_id:"+cmd.listId,
			"item_id:"+cmd.itemId,
		))
}
