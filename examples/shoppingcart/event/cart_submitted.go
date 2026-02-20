package event

type CartSubmitted struct {
	CartId string `json:"cartId"`
}

func (e CartSubmitted) Tags() []string {
	return []string{CartIdTag(e.CartId)}
}
