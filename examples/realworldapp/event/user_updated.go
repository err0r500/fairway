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
		PreviousUserNameTagPrefix(e.PreviousUsername),
	}
}

type UserChangedTheirEmail struct {
	UserId        string `json:"userId"`
	PreviousEmail string `json:"previousEmail"`
	NewEmail      string `json:"newEmail"`
}

func (e UserChangedTheirEmail) Tags() []string {
	return []string{
		UserIdTagPrefix(e.UserId),
		UserEmailTagPrefix(e.NewEmail),
		PreviousUserEmailTagPrefix(e.PreviousEmail),
	}
}

type UserChangedTheirPassword struct {
	UserId            string `json:"userId"`
	NewHashedPassword string `json:"newHashedPassword"`
}

func (e UserChangedTheirPassword) Tags() []string {
	return []string{UserIdTagPrefix(e.UserId)}
}

type UserChangedDetails struct {
	UserId string  `json:"userId"`
	Bio    *string `json:"bio,omitempty"`
	Image  *string `json:"image,omitempty"`
}

func (e UserChangedDetails) Tags() []string {
	return []string{UserIdTagPrefix(e.UserId)}
}
