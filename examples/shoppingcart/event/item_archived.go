package event

type ItemArchived struct {
	CartId string `json:"cartId"`
	ItemId string `json:"itemId"`
}

func (e ItemArchived) Tags() []string {
	return []string{
		CartIdTag(e.CartId),
		ItemIdTag(e.ItemId),
	}
}
