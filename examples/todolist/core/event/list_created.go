package event

type ListCreated struct {
	ListId string `json:"listId"`
	Name   string `json:"name"`
}
