package event

type UserWelcomeEmailSent struct {
	UserId string `json:"userId"`
}

func (e UserWelcomeEmailSent) Tags() []string {
	return []string{UserIdTag(e.UserId)}
}
