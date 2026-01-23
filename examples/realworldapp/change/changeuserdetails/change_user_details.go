package changeuserdetails

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/utils"
)

func init() {
	Register(&change.ChangeRegistry)
}

func Register(registry *fairway.HttpChangeRegistry) {
	jwt := crypto.NewJwtService(os.Getenv("JWT_SECRET"))
	registry.RegisterCommand("PUT /user/details", httpHandler(jwt))
}

var notFoundErr = errors.New("user not found")

type reqBody struct {
	Bio   string `json:"bio"`
	Image string `json:"image"`
}

func httpHandler(jwtService crypto.JwtService) func(runner fairway.CommandRunner) http.HandlerFunc {
	return func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, err := jwtService.ExtractUserID(r)
			if err != nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			var req reqBody
			if err := utils.JsonParse(r, &req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(err.Error())
				return
			}

			if err := runner.RunPure(r.Context(), command{
				userID: userID,
				bio:    req.Bio,
				image:  req.Image,
			}); err != nil {
				if errors.Is(err, notFoundErr) {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(err.Error())
				return
			}

			w.WriteHeader(http.StatusNoContent)
		}
	}
}

type command struct {
	userID string
	bio    string
	image  string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var userExists bool
	var currentBio, currentImage string

	if err := ev.ReadEvents(ctx, fairway.QueryItems(
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedDetails{}).
			Tags(event.UserIdTagPrefix(cmd.userID)),
	), func(te fairway.TaggedEvent) bool {
		switch e := te.(type) {
		case event.UserRegistered:
			if e.Id == cmd.userID {
				userExists = true
			}
		case event.UserChangedDetails:
			if e.UserId == cmd.userID {
				if e.Bio != nil {
					currentBio = *e.Bio
				}
				if e.Image != nil {
					currentImage = *e.Image
				}
			}
		}
		return true
	}); err != nil {
		return err
	}

	if !userExists {
		return notFoundErr
	}

	if cmd.bio == currentBio && cmd.image == currentImage {
		return nil // no changes
	}

	bio := cmd.bio
	image := cmd.image
	return ev.AppendEvents(ctx, event.UserChangedDetails{
		UserId: cmd.userID,
		Bio:    &bio,
		Image:  &image,
	})
}
