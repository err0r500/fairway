package event

func UserIdTagPrefix(id string) string {
	return "user_id:" + id
}

func UserNameTagPrefix(name string) string {
	return "username:" + name
}

func UserEmailTagPrefix(email string) string {
	return "email:" + email
}

func PreviousUserNameTagPrefix(name string) string {
	return "previous_username:" + name
}

func PreviousUserEmailTagPrefix(email string) string {
	return "previous_email:" + email
}
