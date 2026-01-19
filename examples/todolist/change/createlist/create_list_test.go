package createlist_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/todolist/change/createlist"
	"github.com/err0r500/fairway/examples/todolist/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/testing/then"
	"github.com/stretchr/testify/assert"
)

func TestCreateList_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, createlist.Register)

	// Given
	listId := "list-1"
	listName := "list name"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"Name": listName}).
		Post(apiRoute(server, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())
	assert.Empty(t, string(resp.Bytes()))
	then.ExpectEventsInStore(t, store, event.ListCreated{ListId: listId, Name: listName})
}

func TestCreateList_Conflict(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, createlist.Register)

	// Given
	listId := "list-1"
	initialEvent := event.ListCreated{ListId: listId, Name: "list 1 name"}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"name": "anything"}).
		Post(apiRoute(server, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestCreateList_ApiValidation(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, createlist.Register)

	// Given
	listId := "list-1"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{}).
		Post(apiRoute(server, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
	then.ExpectEventsInStore(t, store)
}

func apiRoute(server *httptest.Server, listId string) string {
	return server.URL + "/api/lists/" + listId
}
