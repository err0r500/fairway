package event

type PriceChanged struct {
	ProductId string `json:"productId"`
	OldPrice  int    `json:"oldPrice"`
	NewPrice  int    `json:"newPrice"`
}

func (e PriceChanged) Tags() []string {
	return []string{ProductIdTag(e.ProductId)}
}
