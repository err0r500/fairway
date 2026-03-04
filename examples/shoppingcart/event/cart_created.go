package event

type CartCreated struct {
	CartId string `json:"cartId"`
}

func (e CartCreated) Tags() []string {
	return []string{CartIdTag(e.CartId)}
}
