package registeruser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/utils"
)

const emailReleaseDuration = 3 * 24 * time.Hour

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
			now:            time.Now(),
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
	now            time.Time
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	idTaken := false
	// track email ownership: userId -> releasedAt (nil = still owns it)
	emailOwnership := make(map[string]*time.Time)
	// track name ownership: userId -> owns (true = still owns it)
	nameOwnership := make(map[string]bool)

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserIdTagPrefix(cmd.id)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirName{}).
				Tags(event.UserNameTagPrefix(cmd.name)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
				Tags(event.UserEmailTagPrefix(cmd.email)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.UserRegistered:
				if data.Id == cmd.id {
					idTaken = true
					break // if another user registered with this id, no need to see more
				}
				if data.Email == cmd.email {
					emailOwnership[data.Id] = nil // owns it
				}
				if data.Name == cmd.name {
					nameOwnership[data.Id] = true // owns it
				}
			case event.UserChangedTheirEmail:
				if data.NewEmail == cmd.email {
					emailOwnership[data.UserId] = nil // owns it
				} else if data.PreviousEmail == cmd.email {
					releasedAt := e.OccuredAt()
					emailOwnership[data.UserId] = &releasedAt // released it
				}
			case event.UserChangedTheirName:
				if data.NewUsername == cmd.name {
					nameOwnership[data.UserId] = true // owns it
				} else if data.PreviousUsername == cmd.name {
					nameOwnership[data.UserId] = false // released it
				}
			}
			return true
		}); err != nil {
		return err
	}

	if idTaken {
		return conflictErr
	}

	// check if email is available: either never taken, or released >= 3 days ago
	for _, releasedAt := range emailOwnership {
		if releasedAt == nil || releasedAt.After(cmd.now.Add(-emailReleaseDuration)) {
			return conflictErr
		}
	}

	// check if name is available
	for _, owns := range nameOwnership {
		if owns {
			return conflictErr
		}
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.UserRegistered{
		Id:             cmd.id,
		Name:           cmd.name,
		Email:          cmd.email,
		HashedPassword: cmd.hashedPassword,
	}))
}
