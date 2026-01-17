package event

type TodoItemAdded struct {
	ListId string `json:"listId"`
	ItemId string `json:"itemId"`
	Text   string `json:"text"`
}
