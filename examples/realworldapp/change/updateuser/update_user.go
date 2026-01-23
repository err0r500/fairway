package updateuser

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

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
	registry.RegisterCommand("PUT /user", httpHandler(jwt))
}

var (
	conflictErr = errors.New("username or email already taken")
	notFoundErr = errors.New("user not found")
)

type reqBody struct {
	Username string `json:"username" validate:"required"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
	Bio      string `json:"bio"`
	Image    string `json:"image"`
}

func httpHandler(jwtService crypto.JwtService) func(runner fairway.CommandRunner) http.HandlerFunc {
	return func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, err := extractUserID(r, jwtService)
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
				bio:            req.Bio,
				image:          req.Image,
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
}

func extractUserID(r *http.Request, jwtService crypto.JwtService) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Token" {
		return "", errors.New("invalid authorization header")
	}

	return jwtService.Validate(parts[1])
}

type command struct {
	userID         string
	username       string
	email          string
	hashedPassword string
	bio            string
	image          string
}

func (cmd command) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	var userExists bool
	var currentUsername, currentEmail, currentBio, currentImage string

	// Query user + potential conflicts
	queryItems := []fairway.QueryItem{
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirName{}, event.UserChangedTheirEmail{}, event.UserChangedDetails{}).
			Tags(event.UserIdTagPrefix(cmd.userID)),
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirName{}).
			Tags(event.UserNameTagPrefix(cmd.username)),
		fairway.NewQueryItem().
			Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
			Tags(event.UserEmailTagPrefix(cmd.email)),
	}

	usernameConflict := false
	emailConflict := false

	if err := ev.ReadEvents(ctx, fairway.QueryItems(queryItems...),
		func(te fairway.TaggedEvent) bool {
			switch e := te.(type) {
			case event.UserRegistered:
				if e.Id == cmd.userID {
					userExists = true
					currentUsername = e.Name
					currentEmail = e.Email
				} else {
					if e.Name == cmd.username {
						usernameConflict = true
					}
					if e.Email == cmd.email {
						emailConflict = true
					}
				}
			case event.UserChangedTheirName:
				if e.UserId == cmd.userID {
					currentUsername = e.NewUsername
				} else if e.NewUsername == cmd.username {
					usernameConflict = true
				}
			case event.UserChangedTheirEmail:
				if e.UserId == cmd.userID {
					currentEmail = e.NewEmail
				} else if e.NewEmail == cmd.email {
					emailConflict = true
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

	// Check conflicts only if value changed
	if cmd.username != currentUsername && usernameConflict {
		return conflictErr
	}
	if cmd.email != currentEmail && emailConflict {
		return conflictErr
	}

	// Emit events for changes
	var events []fairway.TaggedEvent

	if cmd.username != currentUsername {
		events = append(events, event.UserChangedTheirName{
			UserId:      cmd.userID,
			NewUsername: cmd.username,
		})
	}

	if cmd.email != currentEmail {
		events = append(events, event.UserChangedTheirEmail{
			UserId:   cmd.userID,
			NewEmail: cmd.email,
		})
	}

	// Password: always emit since we can't compare hashes
	events = append(events, event.UserChangedTheirPassword{
		UserId:            cmd.userID,
		NewHashedPassword: cmd.hashedPassword,
	})

	if cmd.bio != currentBio || cmd.image != currentImage {
		bio := cmd.bio
		image := cmd.image
		events = append(events, event.UserChangedDetails{
			UserId: cmd.userID,
			Bio:    &bio,
			Image:  &image,
		})
	}

	return ev.AppendEvents(ctx, events...)
}
