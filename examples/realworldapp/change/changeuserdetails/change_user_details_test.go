package changeuserdetails_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change/changeuserdetails"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestChangeUserDetails_CanUpdateUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"username": "newname",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CannotUpdateUsernameToTaken(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	takenUsername := "taken"
	given.EventsInStore(store,
		fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserRegistered{Id: "user-2", Name: takenUsername, Email: "other@example.com", HashedPassword: "h"}),
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"username": takenUsername,
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeUserDetails_CanUseReleasedUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	otherUserId := "user-2"
	releasedUsername := "oldname"
	given.EventsInStore(store,
		fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserRegistered{Id: otherUserId, Name: releasedUsername, Email: "other@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserChangedTheirName{UserId: otherUserId, PreviousUsername: releasedUsername, NewUsername: "newname"}),
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"username": releasedUsername,
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CanUpdateBio(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"bio": "My new bio",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_CanUpdateImage(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"image": "https://example.com/avatar.png",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeUserDetails_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)

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
	_, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	nonexistentUserId := "nonexistent"

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, nonexistentUserId)).
		SetBody(map[string]any{
			"bio": "bio",
		}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode())
}

func TestChangeUserDetails_EmptyBodySucceeds(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeuserdetails.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{}).
		Patch(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func generateToken(t *testing.T, userID string) string {
	token, err := crypto.JwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user/details"
}
