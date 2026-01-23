package registeruser_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/realworldapp/change/registeruser"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/testing/then"
	"github.com/stretchr/testify/assert"
)

func TestRegisterUser_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// Given
	userId := "user-1"
	username := "johndoe"
	email := "john@example.com"
	password := "secret123"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       userId,
			"username": username,
			"email":    email,
			"password": password,
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())
	assert.Empty(t, string(resp.Bytes()))

	// custom verification because of hashing
	var stored event.UserRegistered
	count := 0
	for e, err := range store.ReadAll(context.Background()) {
		assert.NoError(t, err)
		count++
		assert.NoError(t, json.Unmarshal(e.Data, &stored))
	}
	assert.Equal(t, 1, count)
	assert.Equal(t, userId, stored.Id)
	assert.Equal(t, username, stored.Name)
	assert.Equal(t, email, stored.Email)
	assert.True(t, crypto.HashMatchesCleartext(stored.HashedPassword, password))
}

func TestRegisterUser_ConflictById(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// Given
	userId := "user-1"
	initialEvent := event.UserRegistered{Id: userId, Name: "existing", Email: "existing@example.com", HashedPassword: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       userId,
			"username": "newuser",
			"email":    "new@example.com",
			"password": "newpass",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestRegisterUser_ConflictByEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// Given
	email := "taken@example.com"
	initialEvent := event.UserRegistered{Id: "user-1", Name: "existing", Email: email, HashedPassword: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       "user-2",
			"username": "newuser",
			"email":    email,
			"password": "newpass",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestRegisterUser_ConflictByUsername(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// Given
	username := "takenuser"
	initialEvent := event.UserRegistered{Id: "user-1", Name: username, Email: "existing@example.com", HashedPassword: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       "user-2",
			"username": username,
			"email":    "new@example.com",
			"password": "newpass",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestRegisterUser_ApiValidation(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
	then.ExpectEventsInStore(t, store)
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/users"
}
