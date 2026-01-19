package event

type ItemAdded struct {
	ListId string `json:"listId"`
	ItemId string `json:"itemId"`
	Text   string `json:"text"`
}

func (e ItemAdded) Tags() []string {
	return []string{
		ListTagPrefix(e.ListId),
		ItemTagPrefix(e.ItemId),
	}
}
