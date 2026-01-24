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
	registry.RegisterCommand("PATCH /user/auth", httpHandler)
}

var (
	conflictErr = errors.New("email already taken")
	notFoundErr = errors.New("user not found")
)

type reqBody struct {
	Email    *string `json:"email" validate:"omitempty,email"`
	Password *string `json:"password"`
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
			userID:            userID,
			email:             req.Email,
			cleartextPassword: req.Password,
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
	userID            string
	email             *string
	cleartextPassword *string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var currentEmail *string
	otherHasEmail := make(map[string]bool)

	queryItems := []fairway.QueryItem{
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
			Tags(event.UserIdTagPrefix(cmd.userID)),
	}
	if cmd.email != nil {
		queryItems = append(queryItems,
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
				Tags(event.UserEmailTagPrefix(*cmd.email)),
		)
	}

	if err := ev.ReadEvents(ctx, fairway.QueryItems(queryItems...), func(e fairway.Event) bool {
		switch data := e.Data.(type) {
		case event.UserRegistered:
			if data.Id == cmd.userID {
				currentEmail = &data.Email
				break
			}
			if cmd.email != nil && data.Email == *cmd.email {
				otherHasEmail[data.Id] = true
			}
		case event.UserChangedTheirEmail:
			if data.UserId == cmd.userID {
				currentEmail = &data.NewEmail
				break
			}
			if cmd.email != nil {
				if data.NewEmail == *cmd.email {
					otherHasEmail[data.UserId] = true
				} else if data.PreviousEmail == *cmd.email {
					otherHasEmail[data.UserId] = false
				}
			}
		}
		return true
	}); err != nil {
		return err
	}

	if currentEmail == nil {
		return notFoundErr
	}

	for _, has := range otherHasEmail {
		if has {
			return conflictErr
		}
	}

	var events []fairway.Event

	if cmd.email != nil {
		events = append(events, fairway.NewEvent(event.UserChangedTheirEmail{
			UserId:        cmd.userID,
			PreviousEmail: *currentEmail,
			NewEmail:      *cmd.email,
		}))
	}

	if cmd.cleartextPassword != nil {
		events = append(events, fairway.NewEvent(event.UserChangedTheirPassword{
			UserId:            cmd.userID,
			NewHashedPassword: crypto.Hash(*cmd.cleartextPassword),
		}))
	}

	if len(events) == 0 {
		return nil
	}

	return ev.AppendEvents(ctx, events...)
}
