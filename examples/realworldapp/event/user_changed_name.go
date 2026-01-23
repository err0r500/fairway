package event

type UserChangedTheirName struct {
	UserId           string `json:"userId"`
	PreviousUsername string `json:"previousUsername"`
	NewUsername      string `json:"newUsername"`
}

func (e UserChangedTheirName) Tags() []string {
	return []string{
		UserIdTagPrefix(e.UserId),
		UserNameTagPrefix(e.NewUsername),
		UserNameTagPrefix(e.PreviousUsername),
	}
}
