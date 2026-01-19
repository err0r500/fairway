package event

type ListCreated struct {
	ListId string `json:"listId"`
	Name   string `json:"name"`
}

func (e ListCreated) Tags() []string {
	return []string{
		TagListId(e.ListId),
	}
}
