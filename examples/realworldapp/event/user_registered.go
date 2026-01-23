package event

import (
	"log"

	"golang.org/x/crypto/bcrypt"
)

type UserRegistered struct {
	Id       string `json:"id"`
	Name     string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"Password"`
}

func NewUserRegistered(id, name, email, plainPassword string) UserRegistered {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	return UserRegistered{Id: id, Name: name, Email: email, Password: string(hashed)}
}

func (e UserRegistered) Tags() []string {
	return []string{
		UserIdTagPrefix(e.Id),
		UserNameTagPrefix(e.Name),
		UserEmailTagPrefix(e.Email),
	}
}
