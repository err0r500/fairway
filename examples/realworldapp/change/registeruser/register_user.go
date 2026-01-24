package registeruser

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
	registry.RegisterCommand("POST /users", httpHandler)
}

var conflictErr = errors.New("a user field conflicts")

type reqBody struct {
	Id       string `json:"id" validate:"required"`
	Username string `json:"username" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// httpHandler creates an HTTP handler for this command
func httpHandler(runner fairway.CommandRunner) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req reqBody
		if err := utils.JsonParse(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		if err := runner.RunPure(r.Context(), command{
			id:             req.Id,
			name:           req.Username,
			email:          req.Email,
			hashedPassword: crypto.Hash(req.Password),
		}); err != nil {
			if errors.Is(err, conflictErr) {
				w.WriteHeader(http.StatusConflict)
				return
			}

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(err.Error())
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

type command struct {
	id             string
	name           string
	email          string
	hashedPassword string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	conflict := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserIdTagPrefix(cmd.id)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserNameTagPrefix(cmd.name)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserEmailTagPrefix(cmd.email)),
		),
		func(e fairway.Event) bool {
			switch e.Data.(type) {
			case event.UserRegistered:
				conflict = true
				return false
			default:
				return true
			}
		}); err != nil {
		return err
	}

	if conflict {
		return conflictErr
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.UserRegistered{
		Id:             cmd.id,
		Name:           cmd.name,
		Email:          cmd.email,
		HashedPassword: cmd.hashedPassword,
	}))
}
