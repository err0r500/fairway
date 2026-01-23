package registeruser_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/realworldapp/change/registeruser"
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
	then.ExpectEventsInStore(t, store, event.UserRegistered{
		Id:       userId,
		Name:     username,
		Email:    email,
		Password: password,
	})
}

func TestRegisterUser_ConflictById(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, registeruser.Register)

	// Given
	userId := "user-1"
	initialEvent := event.UserRegistered{Id: userId, Name: "existing", Email: "existing@example.com", Password: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       userId,
			"username": "other",
			"email":    "other@example.com",
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
	initialEvent := event.UserRegistered{Id: "user-1", Name: "existing", Email: email, Password: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       "other_id",
			"username": "other",
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
	initialEvent := event.UserRegistered{Id: "user-1", Name: username, Email: "existing@example.com", Password: "pass"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       "user-2",
			"username": username,
			"email":    "other@example.com",
			"password": "newpass",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestRegisterUser_MalformedPayload(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty body", map[string]any{}},
		{"missing id", map[string]any{"username": "john", "email": "john@example.com", "password": "pass"}},
		{"missing username", map[string]any{"id": "1", "email": "john@example.com", "password": "pass"}},
		{"missing email", map[string]any{"id": "1", "username": "john", "password": "pass"}},
		{"missing password", map[string]any{"id": "1", "username": "john", "email": "john@example.com"}},
		{"empty id", map[string]any{"id": "", "username": "john", "email": "john@example.com", "password": "pass"}},
		{"empty username", map[string]any{"id": "1", "username": "", "email": "john@example.com", "password": "pass"}},
		{"empty email", map[string]any{"id": "1", "username": "john", "email": "", "password": "pass"}},
		{"empty password", map[string]any{"id": "1", "username": "john", "email": "john@example.com", "password": ""}},
		{"invalid email - no @", map[string]any{"id": "1", "username": "john", "email": "invalid", "password": "pass"}},
		{"invalid email - no domain", map[string]any{"id": "1", "username": "john", "email": "john@", "password": "pass"}},
		{"invalid email - no local", map[string]any{"id": "1", "username": "john", "email": "@example.com", "password": "pass"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, server, httpClient := given.FreshSetup(t, registeruser.Register)

			resp, err := httpClient.R().
				SetBody(tc.body).
				Post(apiRoute(server))

			assert.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
			then.ExpectEventsInStore(t, store)
		})
	}
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/users"
}
