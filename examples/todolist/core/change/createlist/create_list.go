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
	change.ChangeRegistry.RegisterCommand("POST /api/lists/{listId}", createListHttpHandler)
}

var listAlreadyExistsErr = errors.New("list already exists")

type createListHttpReq struct {
	Name string `json:"name" validate:"required"`
}

// createListHttpHandler creates an HTTP handler for this command
func createListHttpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createListHttpReq
		if !fairway.JsonParse(w, r, &req) {
			return
		}

		if err := runner.RunPure(r.Context(), createList{
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

type createList struct {
	listId string
	name   string
}

func (cmd createList) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	listAlreadyExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().Types(event.ListCreated{}).Tags("list_id:"+cmd.listId),
		),
		func(te fairway.TaggedEvent, _ error) bool {
			switch te.Event.(type) {
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
		fairway.Event(
			event.ListCreated{ListId: cmd.listId, Name: cmd.name},
			"list_id:"+cmd.listId,
		))
}
