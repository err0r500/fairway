package event

type UserChangedTheirEmail struct {
	UserId        string `json:"userId"`
	PreviousEmail string `json:"previousEmail"`
	NewEmail      string `json:"newEmail"`
}

func (e UserChangedTheirEmail) Tags() []string {
	return []string{
		UserIdTagPrefix(e.UserId),
		UserEmailTagPrefix(e.NewEmail),
		UserEmailTagPrefix(e.PreviousEmail),
	}
}
