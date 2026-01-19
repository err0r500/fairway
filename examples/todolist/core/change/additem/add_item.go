package change

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/core/change"
	"github.com/err0r500/fairway/examples/todolist/core/event"
)

func init() {
	change.ChangeRegistry.RegisterCommand("POST /api/lists/{listId}/items/{itemId}", httpHandler)
}

var itemAlreadyExistsErr = errors.New("item already exists")

type reqBody struct {
	Text string `json:"text" validate:"required"`
}

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reqBody
		if !fairway.JsonParse(w, r, &req) {
			return
		}

		if err := runner.RunPure(r.Context(), command{
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

type command struct {
	listId string
	itemId string
	text   string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	itemAlreadyExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.ItemAdded{}).
				Tags(event.ItemTagPrefix(cmd.itemId)),
		),
		func(te fairway.TaggedEvent) bool {
			switch te.(type) {
			case event.ItemAdded:
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
		fairway.TaggedEvent(
			event.ItemAdded{ListId: cmd.listId, ItemId: cmd.itemId, Text: cmd.text},
		))
}
