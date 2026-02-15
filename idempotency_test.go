package fairway_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIdempotencyStore implements dcb.IdempotencyStore for testing
type mockIdempotencyStore struct {
	entries map[string]int
}

func newMockIdempotencyStore() *mockIdempotencyStore {
	return &mockIdempotencyStore{entries: make(map[string]int)}
}

func (m *mockIdempotencyStore) Check(_ context.Context, key string) (int, bool, error) {
	code, found := m.entries[key]
	return code, found, nil
}

func (m *mockIdempotencyStore) Store(_ context.Context, key string, statusCode int) error {
	m.entries[key] = statusCode
	return nil
}

func TestIdempotency_WithoutHeader_PassesThrough(t *testing.T) {
	store := newMockIdempotencyStore()

	handlerCalled := 0
	registry := &fairway.HttpChangeRegistry{}
	registry.WithIdempotency(store)
	registry.RegisterCommand("POST /test", func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
			w.WriteHeader(http.StatusCreated)
		}
	})

	mockRunner := &mockStore{}
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, fairway.NewCommandRunner(mockRunner))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Two requests without Idempotency-Key header
	resp1, err := http.Post(server.URL+"/test", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	resp1.Body.Close()

	resp2, err := http.Post(server.URL+"/test", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	resp2.Body.Close()

	assert.Equal(t, 2, handlerCalled, "handler should be called for each request without idempotency key")
}

func TestIdempotency_WithHeader_DeduplicatesRequests(t *testing.T) {
	store := newMockIdempotencyStore()

	handlerCalled := 0
	registry := &fairway.HttpChangeRegistry{}
	registry.WithIdempotency(store)
	registry.RegisterCommand("POST /test", func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
			w.WriteHeader(http.StatusCreated)
		}
	})

	mockRunner := &mockStore{}
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, fairway.NewCommandRunner(mockRunner))

	server := httptest.NewServer(mux)
	defer server.Close()

	// First request with Idempotency-Key
	req1, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req1.Header.Set("Idempotency-Key", "unique-key-1")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)
	assert.Equal(t, 1, handlerCalled)

	// Second request with same Idempotency-Key
	req2, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req2.Header.Set("Idempotency-Key", "unique-key-1")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusCreated, resp2.StatusCode, "should return cached status code")
	assert.Equal(t, 1, handlerCalled, "handler should NOT be called again for same key")
}

func TestIdempotency_DifferentKeys_BothProcessed(t *testing.T) {
	store := newMockIdempotencyStore()

	handlerCalled := 0
	registry := &fairway.HttpChangeRegistry{}
	registry.WithIdempotency(store)
	registry.RegisterCommand("POST /test", func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
			w.WriteHeader(http.StatusCreated)
		}
	})

	mockRunner := &mockStore{}
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, fairway.NewCommandRunner(mockRunner))

	server := httptest.NewServer(mux)
	defer server.Close()

	// First request
	req1, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req1.Header.Set("Idempotency-Key", "key-a")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()

	// Second request with different key
	req2, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req2.Header.Set("Idempotency-Key", "key-b")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()

	assert.Equal(t, 2, handlerCalled, "different keys should both be processed")
}

func TestIdempotency_CachesErrorStatusCodes(t *testing.T) {
	store := newMockIdempotencyStore()

	handlerCalled := 0
	registry := &fairway.HttpChangeRegistry{}
	registry.WithIdempotency(store)
	registry.RegisterCommand("POST /test", func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
			w.WriteHeader(http.StatusConflict)
		}
	})

	mockRunner := &mockStore{}
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, fairway.NewCommandRunner(mockRunner))

	server := httptest.NewServer(mux)
	defer server.Close()

	// First request - gets 409
	req1, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req1.Header.Set("Idempotency-Key", "conflict-key")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusConflict, resp1.StatusCode)

	// Second request - should return cached 409
	req2, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req2.Header.Set("Idempotency-Key", "conflict-key")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusConflict, resp2.StatusCode, "should return cached error status")
	assert.Equal(t, 1, handlerCalled, "handler should NOT be called again")
}

func TestIdempotency_WithoutStore_NoMiddleware(t *testing.T) {
	handlerCalled := 0
	registry := &fairway.HttpChangeRegistry{}
	// No WithIdempotency call
	registry.RegisterCommand("POST /test", func(runner fairway.CommandRunner) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			handlerCalled++
			w.WriteHeader(http.StatusCreated)
		}
	})

	mockRunner := &mockStore{}
	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, fairway.NewCommandRunner(mockRunner))

	server := httptest.NewServer(mux)
	defer server.Close()

	// Requests with Idempotency-Key but no store configured
	req1, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req1.Header.Set("Idempotency-Key", "key-1")
	resp1, err := http.DefaultClient.Do(req1)
	require.NoError(t, err)
	resp1.Body.Close()

	req2, _ := http.NewRequest("POST", server.URL+"/test", strings.NewReader("{}"))
	req2.Header.Set("Idempotency-Key", "key-1")
	resp2, err := http.DefaultClient.Do(req2)
	require.NoError(t, err)
	resp2.Body.Close()

	assert.Equal(t, 2, handlerCalled, "without store, both requests should be processed")
}
