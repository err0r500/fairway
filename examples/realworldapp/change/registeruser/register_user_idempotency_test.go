package registeruser_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change/registeruser"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterUser_Idempotent_SameKeyReturnsCachedResponse(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetupWithIdempotency(t, registeruser.Register)

	body := map[string]any{
		"id":       "user-idem-1",
		"username": "idempotent",
		"email":    "idem@example.com",
		"password": "secret123",
	}

	// First request - should succeed with 201
	resp1, err := httpClient.R().
		SetHeader("Idempotency-Key", "register-key-1").
		SetBody(body).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp1.StatusCode())

	// Second request with same idempotency key - should return cached 201
	resp2, err := httpClient.R().
		SetHeader("Idempotency-Key", "register-key-1").
		SetBody(body).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode(), "retry with same key should return cached 201")

	// Verify only one event was stored (not two)
	count := 0
	for _, readErr := range store.ReadAll(context.Background()) {
		assert.NoError(t, readErr)
		count++
	}
	assert.Equal(t, 1, count, "should have exactly one event in store")
}

func TestRegisterUser_Idempotent_DifferentKeysProcessBoth(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetupWithIdempotency(t, registeruser.Register)

	// First request
	resp1, err := httpClient.R().
		SetHeader("Idempotency-Key", "key-a").
		SetBody(map[string]any{
			"id":       "user-a",
			"username": "user_a",
			"email":    "a@example.com",
			"password": "pass",
		}).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp1.StatusCode())

	// Second request with different key and different data
	resp2, err := httpClient.R().
		SetHeader("Idempotency-Key", "key-b").
		SetBody(map[string]any{
			"id":       "user-b",
			"username": "user_b",
			"email":    "b@example.com",
			"password": "pass",
		}).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp2.StatusCode())
}

func TestRegisterUser_Idempotent_ConflictIsCached(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetupWithIdempotency(t, registeruser.Register)

	// Setup: register a user first (without idempotency key)
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{
		Id: "existing-user", Name: "taken", Email: "taken@example.com", HashedPassword: "pass",
	}))

	// First request that will conflict
	resp1, err := httpClient.R().
		SetHeader("Idempotency-Key", "conflict-key").
		SetBody(map[string]any{
			"id":       "existing-user",
			"username": "newuser",
			"email":    "new@example.com",
			"password": "pass",
		}).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp1.StatusCode())

	// Retry with same key - should return cached 409
	resp2, err := httpClient.R().
		SetHeader("Idempotency-Key", "conflict-key").
		SetBody(map[string]any{
			"id":       "existing-user",
			"username": "newuser",
			"email":    "new@example.com",
			"password": "pass",
		}).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp2.StatusCode(), "retry with same key should return cached 409")
}

func TestRegisterUser_Idempotent_WithoutKeyStillWorks(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetupWithIdempotency(t, registeruser.Register)

	// Request without idempotency key
	resp, err := httpClient.R().
		SetBody(map[string]any{
			"id":       "user-no-key",
			"username": "nokey",
			"email":    "nokey@example.com",
			"password": "secret",
		}).
		Post(server.URL + "/users")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())

	// Verify event was stored
	var stored event.UserRegistered
	for e, readErr := range store.ReadAll(context.Background()) {
		assert.NoError(t, readErr)
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(e.Data, &envelope))
		assert.NoError(t, json.Unmarshal(envelope.Data, &stored))
	}
	assert.Equal(t, "user-no-key", stored.Id)
}
