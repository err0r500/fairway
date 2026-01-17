package event

type TodoListCreated struct {
	ListId string `json:"listId"`
	Name   string `json:"name"`
}
