package login

import (
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
			func(e fairway.Event) bool {
				if u, ok := e.Data.(event.UserRegistered); ok {
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

		if !crypto.HashMatchesCleartext(foundUser.HashedPassword, req.Password) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		token, err := crypto.JwtService.Token(foundUser.Id)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{Token: token})
	}
}
