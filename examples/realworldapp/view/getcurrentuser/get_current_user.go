package getcurrentuser

import (
	"encoding/json"
	"net/http"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/examples/realworldapp/view"
)

func init() {
	Register(&view.ViewRegistry)
}

func Register(registry *fairway.HttpViewRegistry) {
	registry.RegisterView("GET /user", httpHandler)
}

type respBody struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Bio      string `json:"bio"`
	Image    string `json:"image"`
}

func httpHandler(reader fairway.EventsReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := crypto.JwtService.ExtractUserID(r)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var user *userState
		if err := reader.ReadEvents(r.Context(),
			fairway.QueryItems(
				fairway.NewQueryItem().
					Types(event.UserRegistered{}, event.UserChangedTheirName{}, event.UserChangedTheirEmail{}, event.UserChangedDetails{}).
					Tags(event.UserIdTag(userID)),
			),
			func(e fairway.Event) bool {
				switch data := e.Data.(type) {
				case event.UserRegistered:
					user = &userState{
						email:    data.Email,
						username: data.Name,
					}
				case event.UserChangedTheirName:
					if user != nil {
						user.username = data.NewUsername
					}
				case event.UserChangedTheirEmail:
					if user != nil {
						user.email = data.NewEmail
					}
				case event.UserChangedDetails:
					if user != nil {
						if data.Bio != nil {
							user.bio = *data.Bio
						}
						if data.Image != nil {
							user.image = *data.Image
						}
					}
				}
				return true
			}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		if user == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(respBody{
			Email:    user.email,
			Username: user.username,
			Bio:      user.bio,
			Image:    user.image,
		})
	}
}

type userState struct {
	email    string
	username string
	bio      string
	image    string
}
