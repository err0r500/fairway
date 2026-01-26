package event

type UserRegistered struct {
	Id             string `json:"id"`
	Name           string `json:"username"`
	Email          string `json:"email"`
	HashedPassword string `json:"hashedPassword"`
}

func (e UserRegistered) Tags() []string {
	return []string{
		UserIdTag(e.Id),
		UserNameTag(e.Name),
		UserEmailTag(e.Email),
	}
}
