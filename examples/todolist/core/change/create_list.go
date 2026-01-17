package change

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/core/event"
)

func init() {
	ChangeRegistry.RegisterCommand("POST /api/lists/{listId}", createListHttpHandler)
}

// createListHttpHandler creates an HTTP handler for this command
func createListHttpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listId := r.PathValue("listId")
		var req struct {
			Name string `json:"name" validate:"required"`
		}
		if !fairway.JsonParse(w, r, &req) {
			return
		}

		if err := runner.RunPure(r.Context(), createList{
			listId: listId,
			name:   req.Name,
		}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
	return ev.AppendEvents(ctx,
		fairway.Event(
			event.TodoListCreated{ListId: cmd.listId, Name: cmd.name},
			"list_id:"+cmd.listId,
		))
}
