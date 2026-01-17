package change

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/google/uuid"
)

func init() {
	log.Println("registering add_item")
	ChangeRegistry.RegisterCommand("POST /api/lists/{listId}/items", addItemHttpHandler)
}

func addItemHttpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		listId := r.PathValue("listId")

		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}

		itemId := uuid.New().String()
		if err := runner.RunPure(r.Context(), AddItem{
			ListId: listId,
			ItemId: itemId,
			Text:   req.Text,
		}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"itemId": itemId})
	}
}

type AddItem struct {
	ListId string
	ItemId string
	Text   string
}

func (cmd AddItem) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	return nil
}
