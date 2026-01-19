package event

type ItemCreated struct {
	Id     string `json:"id"`
	ListId string `json:"listId"`
	Text   string `json:"text"`
}

func (e ItemCreated) Tags() []string {
	return []string{
		ListTagPrefix(e.ListId),
		ItemTagPrefix(e.Id),
	}
}
