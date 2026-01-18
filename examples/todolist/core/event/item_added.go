package event

type ItemAdded struct {
	ListId string `json:"listId"`
	ItemId string `json:"itemId"`
	Text   string `json:"text"`
}

func (e ItemAdded) Tags() []string {
	return []string{TagListId(e.ListId), TagItemId(e.ItemId)}
}
