package event

type UserChangedTheirName struct {
	UserId      string `json:"userId"`
	NewUsername string `json:"newUsername"`
}

func (e UserChangedTheirName) Tags() []string {
	return []string{
		UserIdTagPrefix(e.UserId),
		UserNameTagPrefix(e.NewUsername),
	}
}

type UserChangedTheirEmail struct {
	UserId   string `json:"userId"`
	NewEmail string `json:"newEmail"`
}

func (e UserChangedTheirEmail) Tags() []string {
	return []string{
		UserIdTagPrefix(e.UserId),
		UserEmailTagPrefix(e.NewEmail),
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
