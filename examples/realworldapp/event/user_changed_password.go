package event

type UserChangedTheirPassword struct {
	UserId            string `json:"userId"`
	NewHashedPassword string `json:"newHashedPassword"`
}

func (e UserChangedTheirPassword) Tags() []string {
	return []string{UserIdTagPrefix(e.UserId)}
}
