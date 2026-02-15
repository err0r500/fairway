package registeruser_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
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

func TestRegisterUser_Idempotent_ParallelRequestsSameKey(t *testing.T) {
	t.Parallel()
	store, server, _ := given.FreshSetupWithIdempotency(t, registeruser.Register)

	body := map[string]any{
		"id":       "user-parallel-1",
		"username": "parallel_user",
		"email":    "parallel@example.com",
		"password": "secret123",
	}

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)

	type result struct {
		statusCode int
		err        error
	}
	results := make([]result, n)

	for i := range n {
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequest("POST", server.URL+"/users", mustMarshalReader(body))
			if err != nil {
				results[idx] = result{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", "parallel-key-1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results[idx] = result{err: err}
				return
			}
			resp.Body.Close()
			results[idx] = result{statusCode: resp.StatusCode}
		}(i)
	}
	wg.Wait()

	// All requests should have completed without error
	for i, r := range results {
		require.NoError(t, r.err, "request %d failed", i)
	}

	// All responses should have the same status code
	firstStatus := results[0].statusCode
	for i, r := range results[1:] {
		assert.Equal(t, firstStatus, r.statusCode,
			"request %d returned %d, expected %d (same as first)", i+1, r.statusCode, firstStatus)
	}

	// Exactly one event should be in the store (as if a single request succeeded)
	eventCount := 0
	for _, readErr := range store.ReadAll(context.Background()) {
		assert.NoError(t, readErr)
		eventCount++
	}
	assert.Equal(t, 1, eventCount, "should have exactly one event in store, not %d", eventCount)
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

func mustMarshalReader(v any) io.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(b)
}
