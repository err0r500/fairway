package changeuserdetails

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
	registry.RegisterCommand("PATCH /user/details", httpHandler)
}

var (
	conflictErr = errors.New("username already taken")
	notFoundErr = errors.New("user not found")
)

type reqBody struct {
	Username *string `json:"username"`
	Bio      *string `json:"bio"`
	Image    *string `json:"image"`
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
			userID:   userID,
			username: req.Username,
			bio:      req.Bio,
			image:    req.Image,
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
	userID   string
	username *string
	bio      *string
	image    *string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var currentUsername *string
	otherHasUsername := make(map[string]bool)

	queryItems := []fairway.QueryItem{
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirName{}).
			Tags(event.UserIdTagPrefix(cmd.userID)),
	}
	if cmd.username != nil {
		queryItems = append(queryItems,
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirName{}).
				Tags(event.UserNameTagPrefix(*cmd.username)),
		)
	}

	if err := ev.ReadEvents(ctx, fairway.QueryItems(queryItems...), func(te fairway.TaggedEvent) bool {
		switch e := te.(type) {
		case event.UserRegistered:
			if e.Id == cmd.userID {
				currentUsername = &e.Name
				break
			}
			if cmd.username != nil && e.Name == *cmd.username {
				otherHasUsername[e.Id] = true
			}
		case event.UserChangedTheirName:
			if e.UserId == cmd.userID {
				currentUsername = &e.NewUsername
				break
			}
			if cmd.username != nil {
				if e.NewUsername == *cmd.username {
					otherHasUsername[e.UserId] = true
				} else if e.PreviousUsername == *cmd.username {
					otherHasUsername[e.UserId] = false
				}
			}
		}
		return true
	}); err != nil {
		return err
	}

	if currentUsername == nil {
		return notFoundErr
	}

	for _, has := range otherHasUsername {
		if has {
			return conflictErr
		}
	}

	var events []fairway.TaggedEvent

	if cmd.username != nil {
		events = append(events, event.UserChangedTheirName{
			UserId:           cmd.userID,
			PreviousUsername: *currentUsername,
			NewUsername:      *cmd.username,
		})
	}

	if cmd.bio != nil || cmd.image != nil {
		events = append(events, event.UserChangedDetails{
			UserId: cmd.userID,
			Bio:    cmd.bio,
			Image:  cmd.image,
		})
	}

	if len(events) == 0 {
		return nil
	}

	return ev.AppendEvents(ctx, events...)
}
