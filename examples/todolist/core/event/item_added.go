package event

type ItemAdded struct {
	ListId string `json:"listId"`
	ItemId string `json:"itemId"`
	Text   string `json:"text"`
}
