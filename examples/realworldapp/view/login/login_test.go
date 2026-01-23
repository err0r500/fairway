package login_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/examples/realworldapp/view/login"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestLogin_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, login.Register)

	// Given
	userId := "user-1"
	email := "john@example.com"
	password := "secret123"
	given.EventsInStore(store, event.UserRegistered{
		Id:             userId,
		Name:           "johndoe",
		Email:          email,
		HashedPassword: crypto.Hash(password),
	})

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"email":    email,
			"password": password,
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	assert.Contains(t, string(resp.Bytes()), "token")
}

func TestLogin_UserNotFound(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, login.Register)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"email":    "nonexistent@example.com",
			"password": "whatever",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestLogin_WrongPassword(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, login.Register)

	// Given
	email := "john@example.com"
	given.EventsInStore(store, event.UserRegistered{
		Id:             "user-1",
		Name:           "johndoe",
		Email:          email,
		HashedPassword: crypto.Hash("correct-password"),
	})

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"email":    email,
			"password": "wrong-password",
		}).
		Post(apiRoute(server))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestLogin_MalformedPayload(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body map[string]any
	}{
		{"empty body", map[string]any{}},
		{"missing email", map[string]any{"password": "pass"}},
		{"missing password", map[string]any{"email": "john@example.com"}},
		{"empty email", map[string]any{"email": "", "password": "pass"}},
		{"empty password", map[string]any{"email": "john@example.com", "password": ""}},
		{"invalid email", map[string]any{"email": "invalid", "password": "pass"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, server, httpClient := given.FreshSetup(t, login.Register)

			resp, err := httpClient.R().
				SetBody(tc.body).
				Post(apiRoute(server))

			assert.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
		})
	}
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/users/login"
}
