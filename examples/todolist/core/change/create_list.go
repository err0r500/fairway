package change

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/core/event"
)

func init() {
	log.Println("registering create_list")
	ChangeRegistry.RegisterCommand("POST /api/lists/{listId}", createListHttpHandler)
}

// createListHttpHandler creates an HTTP handler for this command
func createListHttpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listId := r.PathValue("listId")
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"listId": listId})
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
