//go:build test

package changeuserdetails_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/change/changeuserdetails"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
	"resty.dev/v3"
)

func TestChangeUserDetails_CanUpdateUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "newname",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CannotUpdateUsernameToTaken(t *testing.T) {
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
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeUserDetails_CanUseReleasedUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
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
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CanUpdateBio(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"bio": "My new bio",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CanUpdateImage(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"image": "https://example.com/avatar.png",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := freshSetup(t)

	resp, err := httpClient.R().
		SetBody(map[string]any{
			"bio": "bio",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestChangeUserDetails_UserNotFoundFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := freshSetup(t)
	token := generateToken(t, "nonexistent")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"bio": "bio",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode())
}

func TestChangeUserDetails_EmptyBodySucceeds(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	given.EventsInStore(store, event.UserRegistered{Id: "user-1", Name: "john", Email: "john@example.com", HashedPassword: "h"})
	token := generateToken(t, "user-1")

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func freshSetup(t *testing.T) (dcb.DcbStore, *httptest.Server, *resty.Client) {
	store := dcb.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)
	mux := http.NewServeMux()

	registry := &fairway.HttpChangeRegistry{}
	changeuserdetails.Register(registry)
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
	return server.URL + "/user/details"
}
