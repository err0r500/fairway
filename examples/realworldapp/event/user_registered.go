package event

type UserRegistered struct {
	Id             string `json:"id"`
	Name           string `json:"username"`
	Email          string `json:"email"`
	HashedPassword string `json:"hashedPassword"`
}

func (e UserRegistered) Tags() []string {
	return []string{
		UserIdTagPrefix(e.Id),
		UserNameTagPrefix(e.Name),
		UserEmailTagPrefix(e.Email),
	}
}
