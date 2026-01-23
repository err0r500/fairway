package crypto

import (
	"golang.org/x/crypto/bcrypt"
)

func Hash(cleartextPassword string) string {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cleartextPassword), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	return string(hashedPassword)
}

func HashMatchesCleartext(hashedPassword, cleartextPassword string) bool {
	return nil == bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(cleartextPassword))
}
