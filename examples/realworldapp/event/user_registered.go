package event

type UserRegistered struct {
	Id       string `json:"id"`
	Name string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (e UserRegistered) Tags() []string {
	return []string{
		UserIdTagPrefix(e.Id),
		UserNameTagPrefix(e.Name),
		UserEmailTagPrefix(e.Email),
	}
}
