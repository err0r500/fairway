package createlist_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/todolist/change/createlist"
	"github.com/err0r500/fairway/examples/todolist/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/testing/then"
	"github.com/stretchr/testify/assert"
	"resty.dev/v3"
)

func TestCreateList_Success(t *testing.T) {
	t.Parallel()
	// Given
	store, server, httpClient := setup(t)
	listId := "list-1"
	listName := "list name"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"Name": listName}).
		Post(server.URL + "/api/lists/" + listId)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())
	assert.Empty(t, string(resp.Bytes()))
	then.ExpectEventsInStore(t, store,
		event.ListCreated{ListId: listId, Name: listName})
}

func TestCreateList_Conflict(t *testing.T) {
	t.Parallel()
	// Given
	store, server, httpClient := setup(t)
	listId := "list-1"
	initialEvent := event.ListCreated{ListId: listId, Name: "list 1 name"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"name": "anything"}).
		Post(server.URL + "/api/lists/" + listId)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestCreateList_ApiValidation(t *testing.T) {
	t.Parallel()
	// Given - store with existing ListCreated event
	store, server, httpClient := setup(t)
	listId := "list-1"

	// When - POST to create duplicate
	resp, err := httpClient.R().
		SetBody(map[string]any{}).
		Post(server.URL + "/api/lists/" + listId)

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
	then.ExpectEventsInStore(t, store)
}

func setup(t *testing.T) (dcb.DcbStore, *httptest.Server, *resty.Client) {
	store := dcb.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)

	registry := &fairway.HttpChangeRegistry{}
	createlist.Register(registry)

	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, runner)

	server := httptest.NewServer(mux)
	httpClient := resty.New()
	t.Cleanup(func() {
		server.Close()
		httpClient.Close()
	})
	return store, server, httpClient

}
