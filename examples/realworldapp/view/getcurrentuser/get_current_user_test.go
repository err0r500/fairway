package getcurrentuser_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/examples/realworldapp/view/getcurrentuser"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestGetCurrentUser_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, getcurrentuser.Register)
	currUserId := "user-1"
	email := "john@example.com"
	username := "johndoe"
	given.EventsInStore(store, event.UserRegistered{
		Id:             currUserId,
		Name:           username,
		Email:          email,
		HashedPassword: "h",
	})

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		Get(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	body := string(resp.Bytes())
	assert.Contains(t, body, email)
	assert.Contains(t, body, username)
}

func TestGetCurrentUser_WithUpdatedDetails(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, getcurrentuser.Register)
	currUserId := "user-1"
	newUsername := "newname"
	newEmail := "new@example.com"
	bio := "My bio"
	image := "https://example.com/avatar.png"
	given.EventsInStore(store,
		event.UserRegistered{Id: currUserId, Name: "oldname", Email: "old@example.com", HashedPassword: "h"},
		event.UserChangedTheirName{UserId: currUserId, PreviousUsername: "oldname", NewUsername: newUsername},
		event.UserChangedTheirEmail{UserId: currUserId, PreviousEmail: "old@example.com", NewEmail: newEmail},
		event.UserChangedDetails{UserId: currUserId, Bio: &bio, Image: &image},
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		Get(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	body := string(resp.Bytes())
	assert.Contains(t, body, newUsername)
	assert.Contains(t, body, newEmail)
	assert.Contains(t, body, bio)
	assert.Contains(t, body, image)
}

func TestGetCurrentUser_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, getcurrentuser.Register)

	resp, err := httpClient.R().
		Get(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func generateToken(t *testing.T, userID string) string {
	token, err := crypto.JwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user"
}
