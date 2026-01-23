//go:build test

package updateuser_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/change/updateuser"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
	"resty.dev/v3"
)

func TestUpdateUser_CanUpdateBio(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: "hashed"}
	given.EventsInStore(store, initialEvent)
	token := generateToken(t, userID)

	// When - all fields required, only bio changes
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
			"password": "secret",
			"bio":      "My new bio",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestUpdateUser_CanUpdateEmailToUnusedEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: "hashed"}
	given.EventsInStore(store, initialEvent)
	token := generateToken(t, userID)

	// When
	newEmail := "newemail@example.com"
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    newEmail,
			"password": "secret",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestUpdateUser_CannotUpdateEmailToTakenEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	takenEmail := "taken@example.com"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: "hashed"}
	otherUserEvent := event.UserRegistered{Id: "user-2", Name: "other", Email: takenEmail, HashedPassword: "hashed"}
	given.EventsInStore(store, initialEvent, otherUserEvent)
	token := generateToken(t, userID)

	// When
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    takenEmail,
			"password": "secret",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestUpdateUser_CannotUpdateUsernameToTakenUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	takenUsername := "takenuser"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: "hashed"}
	otherUserEvent := event.UserRegistered{Id: "user-2", Name: takenUsername, Email: "other@example.com", HashedPassword: "hashed"}
	given.EventsInStore(store, initialEvent, otherUserEvent)
	token := generateToken(t, userID)

	// When
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": takenUsername,
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestUpdateUser_CanChangePassword(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: crypto.Hash("oldpassword")}
	given.EventsInStore(store, initialEvent)
	token := generateToken(t, userID)

	// When
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
			"password": "newpassword",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestUpdateUser_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient, _ := freshSetupWithAuth(t)

	// When - no Authorization header
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
			"password": "secret",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestUpdateUser_MissingRequiredFieldFails(t *testing.T) {
	t.Parallel()
	store, server, httpClient, _ := freshSetupWithAuth(t)

	// Given
	userID := "user-1"
	initialEvent := event.UserRegistered{Id: userID, Name: "john", Email: "john@example.com", HashedPassword: "hashed"}
	given.EventsInStore(store, initialEvent)
	token := generateToken(t, userID)

	// When - missing password
	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+token).
		SetBody(map[string]any{
			"username": "john",
			"email":    "john@example.com",
		}).
		Put(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
}

func freshSetupWithAuth(t *testing.T) (dcb.DcbStore, *httptest.Server, *resty.Client, string) {
	os.Setenv("JWT_SECRET", "testsecret")
	store := dcb.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)
	mux := http.NewServeMux()

	changeRegistry := &fairway.HttpChangeRegistry{}
	updateuser.Register(changeRegistry)
	changeRegistry.RegisterRoutes(mux, runner)

	server := httptest.NewServer(mux)
	httpClient := resty.New()
	t.Cleanup(func() {
		server.Close()
		httpClient.Close()
	})
	return store, server, httpClient, ""
}

func generateToken(t *testing.T, userID string) string {
	jwtService := crypto.NewJwtService("testsecret")
	token, err := jwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user"
}
