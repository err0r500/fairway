package createlist

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
	Register(&change.ChangeRegistry)
}

func Register(registry *fairway.HttpChangeRegistry) {
	registry.RegisterCommand("POST /api/lists/{listId}", httpHandler)
}

var listAlreadyExistsErr = errors.New("list already exists")

type reqBody struct {
	Name string `json:"name" validate:"required"`
}

// httpHandler creates an HTTP handler for this command
func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reqBody
		if !fairway.JsonParse(w, r, &req) {
			return
		}

		if err := runner.RunPure(r.Context(), command{
			listId: r.PathValue("listId"),
			name:   req.Name,
		}); err != nil {
			switch err {
			case listAlreadyExistsErr:
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
	name   string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	listAlreadyExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.ListCreated{}).
				Tags(event.ListTagPrefix(cmd.listId)),
		),
		func(te fairway.TaggedEvent) bool {
			switch te.(type) {
			case event.ListCreated:
				listAlreadyExists = true
				return false
			default:
				return true
			}
		}); err != nil {
		return err
	}

	if listAlreadyExists {
		return listAlreadyExistsErr
	}

	return ev.AppendEvents(ctx,
		fairway.TaggedEvent(
			event.ListCreated{ListId: cmd.listId, Name: cmd.name},
		))
}
