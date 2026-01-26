package changepassword

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
	registry.RegisterCommand("PUT /user/password", httpHandler)
}

var notFoundErr = errors.New("user not found")

type reqBody struct {
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
			userID:            userID,
			cleartextPassword: req.Password,
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

type command struct {
	userID            string
	cleartextPassword string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	userExists := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserIdTag(cmd.userID)),
		),
		func(e fairway.Event) bool {
			if data, ok := e.Data.(event.UserRegistered); ok && data.Id == cmd.userID {
				userExists = true
				return false
			}
			return true
		},
	); err != nil {
		return err
	}

	if !userExists {
		return notFoundErr
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.UserChangedTheirPassword{
		UserId:            cmd.userID,
		NewHashedPassword: crypto.Hash(cmd.cleartextPassword),
	}))
}
