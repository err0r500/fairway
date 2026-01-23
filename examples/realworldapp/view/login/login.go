package login

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/examples/realworldapp/view"
	"github.com/err0r500/fairway/utils"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	Register(&view.ViewRegistry)
}

func Register(registry *fairway.HttpViewRegistry) {
	registry.RegisterReadModel("POST /users/login", httpHandler)
}

type reqBody struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type respBody struct {
	Token string `json:"token"`
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reqBody
		if err := utils.JsonParse(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		var foundUser *event.UserRegistered
		if err := reader.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.UserRegistered{}).
					Tags(event.UserEmailTagPrefix(req.Email)),
			),
			func(te fairway.TaggedEvent) bool {
				if u, ok := te.(event.UserRegistered); ok {
					foundUser = &u
					return false
				}
				return true
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		if foundUser == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(foundUser.Password), []byte(req.Password)); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": foundUser.Id,
			"exp":     time.Now().Add(24 * time.Hour).Unix(),
		})

		secret := os.Getenv("JWT_SECRET")
		if secret == "" {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode("JWT_SECRET not configured")
			return
		}

		tokenString, err := token.SignedString([]byte(secret))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Token: tokenString})
	}
}
