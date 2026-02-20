package event

type CartCleared struct {
	CartId string `json:"cartId"`
}

func (e CartCleared) Tags() []string {
	return []string{CartIdTag(e.CartId)}
}
