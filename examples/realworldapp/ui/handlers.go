package ui

import (
	"errors"
	"net/http"
	"time"
	"context"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
)

type Handlers struct {
	runner fairway.CommandRunner
	reader fairway.EventsReader
}

func NewHandlers(runner fairway.CommandRunner, reader fairway.EventsReader) *Handlers {
	return &Handlers{runner: runner, reader: reader}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ui/login", h.loginPage)
	mux.HandleFunc("POST /ui/login", h.loginSubmit)
	mux.HandleFunc("GET /ui/register", h.registerPage)
	mux.HandleFunc("GET /ui/profile", h.profilePage)
}

func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	LoginPage().Render(r.Context(), w)
}

func (h *Handlers) loginSubmit(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	var foundUser *event.UserRegistered
	if err := h.reader.ReadEvents(r.Context(),
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserEmailTag(email)),
		),
		func(e fairway.Event) bool {
			if u, ok := e.Data.(event.UserRegistered); ok {
				foundUser = &u
				return false
			}
			return true
		}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		LoginError("Internal error").Render(r.Context(), w)
		return
	}

	if foundUser == nil || !crypto.HashMatchesCleartext(foundUser.HashedPassword, password) {
		w.WriteHeader(http.StatusUnauthorized)
		LoginError("Invalid email or password").Render(r.Context(), w)
		return
	}

	token, err := crypto.JwtService.Token(foundUser.Id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		LoginError("Internal error").Render(r.Context(), w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})
	w.Header().Set("HX-Redirect", "/ui/profile")
}

func (h *Handlers) registerPage(w http.ResponseWriter, r *http.Request) {
	RegisterPage().Render(r.Context(), w)
}

// func (h *Handlers) registerSubmit(w http.ResponseWriter, r *http.Request) {
// 	username := r.FormValue("username")
// 	email := r.FormValue("email")
// 	password := r.FormValue("password")
//
// 	if username == "" || email == "" || password == "" {
// 		w.WriteHeader(http.StatusBadRequest)
// 		RegisterError("All fields are required").Render(r.Context(), w)
// 		return
// 	}
//
// 	id := uuid.New().String()
// 	cmd := registerCmd{
// 		id:             id,
// 		name:           username,
// 		email:          email,
// 		hashedPassword: crypto.Hash(password),
// 		now:            time.Now(),
// 	}
//
// 	if err := h.runner.RunPure(r.Context(), cmd); err != nil {
// 		if errors.Is(err, errConflict) {
// 			w.WriteHeader(http.StatusConflict)
// 			RegisterError("Username or email already taken").Render(r.Context(), w)
// 			return
// 		}
// 		w.WriteHeader(http.StatusInternalServerError)
// 		RegisterError("Internal error").Render(r.Context(), w)
// 		return
// 	}
//
// 	RegisterSuccess().Render(r.Context(), w)
// }

func (h *Handlers) profilePage(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("jwt")
	if err != nil {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return
	}

	userID, err := crypto.JwtService.Validate(cookie.Value)
	if err != nil {
		http.Redirect(w, r, "/ui/login", http.StatusSeeOther)
		return
	}

	var username, email string
	var bio, image *string

	if err := h.reader.ReadEvents(r.Context(),
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedDetails{}).
				Tags(event.UserIdTag(userID)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.UserRegistered:
				username = data.Name
				email = data.Email
			case event.UserChangedDetails:
				if data.Bio != nil {
					bio = data.Bio
				}
				if data.Image != nil {
					image = data.Image
				}
			}
			return true
		}); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		ProfileError("Internal error").Render(r.Context(), w)
		return
	}

	if username == "" {
		ProfileError("User not found").Render(r.Context(), w)
		return
	}

	ProfilePage(username, email, bio, image).Render(r.Context(), w)
}

// registerCmd implements fairway.Command for user registration.
var errConflict = errors.New("conflict")

type registerCmd struct {
	id             string
	name           string
	email          string
	hashedPassword string
	now            time.Time
}

func (cmd registerCmd) Run(ctx context.Context, ev fairway.EventReadAppender) error {
	emailOwnership := make(map[string]*time.Time)
	nameOwnership := make(map[string]bool)
	idTaken := false

	if err := ev.ReadEvents(ctx,
		fairway.QueryItems(
			fairway.NewQueryItem().
				Types(event.UserRegistered{}).
				Tags(event.UserIdTag(cmd.id)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirName{}).
				Tags(event.UserNameTag(cmd.name)),
			fairway.NewQueryItem().
				Types(event.UserRegistered{}, event.UserChangedTheirEmail{}).
				Tags(event.UserEmailTag(cmd.email)),
		),
		func(e fairway.Event) bool {
			switch data := e.Data.(type) {
			case event.UserRegistered:
				if data.Id == cmd.id {
					idTaken = true
					return false
				}
				if data.Email == cmd.email {
					emailOwnership[data.Id] = nil
				}
				if data.Name == cmd.name {
					nameOwnership[data.Id] = true
				}
			case event.UserChangedTheirEmail:
				if data.NewEmail == cmd.email {
					emailOwnership[data.UserId] = nil
				} else if data.PreviousEmail == cmd.email {
					releasedAt := e.OccuredAt()
					emailOwnership[data.UserId] = &releasedAt
				}
			case event.UserChangedTheirName:
				if data.NewUsername == cmd.name {
					nameOwnership[data.UserId] = true
				} else if data.PreviousUsername == cmd.name {
					nameOwnership[data.UserId] = false
				}
			}
			return true
		}); err != nil {
		return err
	}

	if idTaken {
		return errConflict
	}
	for _, releasedAt := range emailOwnership {
		if releasedAt == nil || releasedAt.After(cmd.now.Add(-3*24*time.Hour)) {
			return errConflict
		}
	}
	for _, owns := range nameOwnership {
		if owns {
			return errConflict
		}
	}

	return ev.AppendEvents(ctx, fairway.NewEvent(event.UserRegistered{
		Id:             cmd.id,
		Name:           cmd.name,
		Email:          cmd.email,
		HashedPassword: cmd.hashedPassword,
	}))
}
