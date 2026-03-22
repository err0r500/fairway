package event

type CartClosed struct {
	CartId string `json:"cartId"`
}

func (e CartClosed) Tags() []string {
	return []string{CartIdTag(e.CartId)}
}
