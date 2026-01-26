package event

func UserIdTag(id string) string {
	return "user_id:" + id
}

func UserNameTag(name string) string {
	return "username:" + name
}

func UserEmailTag(email string) string {
	return "email:" + email
}
