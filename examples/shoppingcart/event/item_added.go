package event

type ItemAdded struct {
	CartId      string `json:"cartId"`
	ItemId      string `json:"itemId"`
	ProductId   string `json:"productId"`
	Description string `json:"description"`
	Image       string `json:"image"`
	Price       int    `json:"price"`
	Quantity    int    `json:"quantity"`
}

func (e ItemAdded) Tags() []string {
	return []string{
		CartIdTag(e.CartId),
		ItemIdTag(e.ItemId),
		ProductIdTag(e.ProductId),
	}
}
