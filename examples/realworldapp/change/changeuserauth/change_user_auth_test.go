//go:build test

package changeuserauth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/change/changeuserauth"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
	"resty.dev/v3"
)

func TestChangeUserAuth_CanUpdateEmailToUnusedEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "newemail@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_CannotUpdateEmailToTakenEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store,
		event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "other", Email: "taken@example.com", HashedPassword: "h"},
	)
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "taken@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeUserAuth_CannotUpdateUsernameToTakenUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store,
		event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "taken", Email: "other@example.com", HashedPassword: "h"},
	)
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "taken",
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeUserAuth_CanChangePassword(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
			"password": "newpassword",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := freshSetup(t)

	resp, err := httpClient.R().
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestChangeUserAuth_MissingFieldFails(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
}

func TestChangeUserAuth_CanUseReleasedUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	// user-2 registered with "oldname", then changed to "newname" - "oldname" is now free
	given.EventsInStore(store,
		event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "oldname", Email: "other@example.com", HashedPassword: "h"},
		event.UserChangedTheirName{UserId: "user-2", PreviousUsername: "oldname", NewUsername: "newname"},
	)
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "oldname",
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_CanUseReleasedEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	// user-2 registered with "old@example.com", then changed to "new@example.com"
	given.EventsInStore(store,
		event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "other", Email: "old@example.com", HashedPassword: "h"},
		event.UserChangedTheirEmail{UserId: "user-2", PreviousEmail: "old@example.com", NewEmail: "new@example.com"},
	)
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "old@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_CannotUseTakenUsernameAfterChange(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	// user-2 changed their name TO "targetname" - it's now taken
	given.EventsInStore(store,
		event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "oldname", Email: "other@example.com", HashedPassword: "h"},
		event.UserChangedTheirName{UserId: "user-2", PreviousUsername: "oldname", NewUsername: "targetname"},
	)
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "targetname",
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func freshSetup(t *testing.T) (dcb.DcbStore, *httptest.Server, *resty.Client) {
	os.Setenv("JWT_SECRET", "testsecret")
	store := dcb.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)
	mux := http.NewServeMux()

	registry := &fairway.HttpChangeRegistry{}
	changeuserauth.Register(registry)
	registry.RegisterRoutes(mux, runner)

	server := httptest.NewServer(mux)
	httpClient := resty.New()
	t.Cleanup(func() {
		server.Close()
		httpClient.Close()
	})
	return store, server, httpClient
}

func generateToken(t *testing.T, userID string) string {
	token, err := crypto.JwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user/auth"
}
