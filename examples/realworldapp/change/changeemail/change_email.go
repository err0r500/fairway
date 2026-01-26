package changeemail

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
	registry.RegisterCommand("PUT /user/email", httpHandler)
}

var (
	conflictErr = errors.New("email already taken or not released for 3 days")
	notFoundErr = errors.New("user not found")
)

type reqBody struct {
	Email string `json:"email" validate:"required,email"`
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
			userID: userID,
			email:  req.Email,
			now:    time.Now(),
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
	userID string
	email  string
	now    time.Time
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var currentEmail *string
	// track email ownership: userId -> releasedAt (nil = still owns it)
	emailOwnership := make(map[string]*time.Time)

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
				Tags(event.UserIdTag(cmd.userID)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
				Tags(event.UserEmailTag(cmd.email)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.UserRegistered:
				if data.Id == cmd.userID {
					currentEmail = &data.Email
				}
				if data.Email == cmd.email && data.Id != cmd.userID {
					emailOwnership[data.Id] = nil // owns it
				}
			case event.UserChangedTheirEmail:
				if data.UserId == cmd.userID {
					currentEmail = &data.NewEmail
				}
				if data.UserId != cmd.userID {
					if data.NewEmail == cmd.email {
						emailOwnership[data.UserId] = nil // owns it
					} else if data.PreviousEmail == cmd.email {
						releasedAt := e.OccuredAt()
						emailOwnership[data.UserId] = &releasedAt // released it
					}
				}
			}
			return true
		},
	); err != nil {
		return err
	}

	if currentEmail == nil {
		return notFoundErr
	}

	// check if email is available: either never taken, or released >= 3 days ago
	for _, releasedAt := range emailOwnership {
		if releasedAt == nil {
			// someone still owns this email
			return conflictErr
		}
		if releasedAt.After(time.Now().Add(emailReleaseDuration * -1)) {
			// released but not long enough ago
			return conflictErr
		}
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.UserChangedTheirEmail{
		UserId:        cmd.userID,
		PreviousEmail: *currentEmail,
		NewEmail:      cmd.email,
	}))
}
