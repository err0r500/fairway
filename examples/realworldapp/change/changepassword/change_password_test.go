package changepassword_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change/changepassword"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestChangePassword_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changepassword.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"password": "newpassword",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangePassword_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, changepassword.Register)

	resp, err := httpClient.R().
		SetBody(map[string]any{
			"password": "newpassword",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestChangePassword_UserNotFound(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, changepassword.Register)
	unknownUserId := "unknown-user"

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, unknownUserId)).
		SetBody(map[string]any{
			"password": "newpassword",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode())
}

func generateToken(t *testing.T, userID string) string {
	token, err := crypto.JwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user/password"
}
