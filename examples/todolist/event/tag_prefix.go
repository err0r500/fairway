package event

func ListTagPrefix(listId string) string {
	return "list_id:" + listId
}

func ItemTagPrefix(itemId string) string {
	return "item_id:" + itemId
}
