package changeuserauth_test

import (
	"net/http"
	"net/http/httptest"
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

func TestChangeUserAuth_CanUpdateEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	currUserId := "user-1"
	given.EventsInStore(store, event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"})

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": "newemail@example.com",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_CannotUpdateEmailToTakenEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	currUserId := "user-1"
	takenEmail := "taken@example.com"
	given.EventsInStore(store,
		event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: "user-2", Name: "other", Email: takenEmail, HashedPassword: "h"},
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": takenEmail,
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeUserAuth_CanChangePassword(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	currUserId := "user-1"
	given.EventsInStore(store, event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"})

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"password": "newpassword",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := freshSetup(t)

	resp, err := httpClient.R().
		SetBody(map[string]any{
			"email": "john@example.com",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestChangeUserAuth_CanUseReleasedEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	currUserId := "user-1"
	otherUserId := "user-2"
	releasedEmail := "old@example.com"
	given.EventsInStore(store,
		event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"},
		event.UserRegistered{Id: otherUserId, Name: "other", Email: releasedEmail, HashedPassword: "h"},
		event.UserChangedTheirEmail{UserId: otherUserId, PreviousEmail: releasedEmail, NewEmail: "new@example.com"},
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": releasedEmail,
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserAuth_EmptyBodySucceeds(t *testing.T) {
	t.Parallel()
	store, server, httpClient := freshSetup(t)
	currUserId := "user-1"
	given.EventsInStore(store, event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"})

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
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
