package changeuserauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

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
	registry.RegisterCommand("PUT /user/auth", httpHandler)
}

var (
	conflictErr = errors.New("username or email already taken")
	notFoundErr = errors.New("user not found")
)

type reqBody struct {
	Username string `json:"username" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := crypto.JwtService.ExtractUserID(r)
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
			userID:         userID,
			username:       req.Username,
			email:          req.Email,
			hashedPassword: crypto.Hash(req.Password),
		}); err != nil {
			if errors.Is(err, conflictErr) {
				w.WriteHeader(http.StatusConflict)
				return
			}
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

type command struct {
	userID         string
	username       string
	email          string
	hashedPassword string
}

type currentUserState struct {
	username string
	email    string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var currentUser *currentUserState
	otherHasUsername := make(map[string]bool) // other userId -> currently has target username
	otherHasEmail := make(map[string]bool)    // other userId -> currently has target email

	if err := ev.ReadEvents(ctx, fairway.QueryItems(
		// current user's events
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirName{}, event.UserChangedTheirEmail{}).
			Tags(event.UserIdTagPrefix(cmd.userID)),
		// events touching target username
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirName{}).
			Tags(event.UserNameTagPrefix(cmd.username)),
		// events touching target email
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
			Tags(event.UserEmailTagPrefix(cmd.email)),
	), func(te fairway.TaggedEvent) bool {
		switch e := te.(type) {
		case event.UserRegistered:
			if e.Id == cmd.userID {
				currentUser = &currentUserState{username: e.Name, email: e.Email}
				break
			}

			if e.Name == cmd.username {
				otherHasUsername[e.Id] = true
			}
			if e.Email == cmd.email {
				otherHasEmail[e.Id] = true
			}
		case event.UserChangedTheirName:
			if e.UserId == cmd.userID {
				currentUser.username = e.NewUsername
				break
			}

			if e.NewUsername == cmd.username {
				otherHasUsername[e.UserId] = true
			} else if e.PreviousUsername == cmd.username {
				otherHasUsername[e.UserId] = false
			}
		case event.UserChangedTheirEmail:
			if e.UserId == cmd.userID {
				currentUser.email = e.NewEmail
				break
			}

			if e.NewEmail == cmd.email {
				otherHasEmail[e.UserId] = true
			} else if e.PreviousEmail == cmd.email {
				otherHasEmail[e.UserId] = false
			}
		}
		return true
	}); err != nil {
		return err
	}

	if currentUser == nil {
		return notFoundErr
	}

	for _, has := range otherHasUsername {
		if has {
			return conflictErr
		}
	}
	for _, has := range otherHasEmail {
		if has {
			return conflictErr
		}
	}

	var events []fairway.TaggedEvent

	if cmd.username != currentUser.username {
		events = append(events, event.UserChangedTheirName{
			UserId:           cmd.userID,
			PreviousUsername: currentUser.username,
			NewUsername:      cmd.username,
		})
	}

	if cmd.email != currentUser.email {
		events = append(events, event.UserChangedTheirEmail{
			UserId:        cmd.userID,
			PreviousEmail: currentUser.email,
			NewEmail:      cmd.email,
		})
	}

	events = append(events, event.UserChangedTheirPassword{
		UserId:            cmd.userID,
		NewHashedPassword: cmd.hashedPassword,
	})

	return ev.AppendEvents(ctx, events...)
}
