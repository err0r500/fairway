package event

type ItemRemoved struct {
	CartId string `json:"cartId"`
	ItemId string `json:"itemId"`
}

func (e ItemRemoved) Tags() []string {
	return []string{
		CartIdTag(e.CartId),
		ItemIdTag(e.ItemId),
	}
}
