package login

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/examples/realworldapp/view"
	"github.com/err0r500/fairway/utils"
)

func init() {
	Register(&view.ViewRegistry)
}

func Register(registry *fairway.HttpViewRegistry) {
	registry.RegisterView("POST /users/login", httpHandler)
}

type reqBody struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type respBody struct {
	Token string `json:"token"`
}

// Login authenticates a user by email/password and returns a JWT token.
// Returns ("", nil) if credentials are invalid.
func Login(ctx context.Context, reader fairway.EventsReader, email, password string) (string, error) {
	var foundUser *event.UserRegistered
	if err := reader.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserEmailTag(email)),
		),
		func(e fairway.Event) bool {
			if u, ok := e.Data.(event.UserRegistered); ok {
				foundUser = &u
				return false
			}
			return true
		}); err != nil {
		return "", err
	}

	if foundUser == nil || !crypto.HashMatchesCleartext(foundUser.HashedPassword, password) {
		return "", nil
	}

	return crypto.JwtService.Token(foundUser.Id)
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reqBody
		if err := utils.JsonParse(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		token, err := Login(r.Context(), reader, req.Email, req.Password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}
		if token == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Token: token})
	}
}
