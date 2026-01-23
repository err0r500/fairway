package crypto

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var JwtService JwtServiceS

func init() {
	JwtService = newJwtService(os.Getenv("JWT_SECRET"))
}

type JwtServiceS struct {
	secret string
}

func newJwtService(secret string) JwtServiceS {
	if secret == "" {
		panic("JwtService: no secret provided")
	}

	return JwtServiceS{secret: secret}
}

func (s JwtServiceS) Token(userId string) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userId,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}).SignedString([]byte(s.secret))
}

func (s JwtServiceS) Validate(tokenString string) (string, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(s.secret), nil
	})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", jwt.ErrSignatureInvalid
	}

	userId, ok := claims["user_id"].(string)
	if !ok {
		return "", jwt.ErrSignatureInvalid
	}

	return userId, nil
}

func (s JwtServiceS) ExtractUserID(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Token" {
		return "", errors.New("invalid authorization header")
	}

	return s.Validate(parts[1])
}
