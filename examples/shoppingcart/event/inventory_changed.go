package event

type InventoryChanged struct {
	ProductId string `json:"productId"`
	Inventory int    `json:"inventory"`
}

func (e InventoryChanged) Tags() []string {
	return []string{ProductIdTag(e.ProductId)}
}
