package crypto

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JwtService struct {
	secret string
}

func NewJwtService(secret string) JwtService {
	if secret == "" {
		panic("no secret")
	}

	return JwtService{secret: secret}
}

func (s JwtService) Token(userId string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userId,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}).SignedString([]byte(s.secret))
}
