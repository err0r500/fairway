package change

import (
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/core/event"
	"github.com/err0r500/fairway/examples/todolist/core/view"
)

func init() {
	view.ViewRegistry.RegisterReadModel("GET /api/lists/{listId}", addItemHttpHandler)
}

type showList struct {
	Id         string `json:"id"`
	Name       string `json:"name"`
	ItemsCount uint   `json:"itemsCount"`
}

func addItemHttpHandler(runner fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := showList{}
		if err := runner.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.ListCreated{}, event.ItemAdded{}).
					Tags(event.TagListId(r.PathValue("listId"))),
			),
			func(te fairway.TaggedEvent, _ error) bool {
				switch e := te.Event.(type) {
				case event.ListCreated:
					list.Id = e.ListId
					list.Name = e.Name
				case event.ItemAdded:
					list.ItemsCount += 1
				}
				return true
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(list)
	}

}
