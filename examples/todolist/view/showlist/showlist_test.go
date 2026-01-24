package showlist_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/todolist/event"
	"github.com/err0r500/fairway/examples/todolist/view/showlist"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestGetList_Success_NoItem(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, showlist.Register)

	// Given
	listId := "list-1"
	listName := "list name"
	given.EventsInStore(store, fairway.NewEvent(event.ListCreated{ListId: listId, Name: listName}))

	// When
	resp, err := httpClient.R().
		Get(apiRoute(server, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	respBody := map[string]any{}
	json.Unmarshal(resp.Bytes(), &respBody)

	assert.Equal(t, map[string]any{
		"id":         listId,
		"name":       listName,
		"itemsCount": float64(0),
	}, respBody)
}

func TestGetList_Success_2Items(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, showlist.Register)

	// Given
	listId := "list-1"
	listName := "list name"
	given.EventsInStore(store,
		fairway.NewEvent(event.ListCreated{ListId: listId, Name: listName}),
		fairway.NewEvent(event.ItemCreated{ListId: listId, Id: "1", Text: "i1-text"}),
		fairway.NewEvent(event.ItemCreated{ListId: listId, Id: "2", Text: "i2-text"}),
	)

	// When
	resp, err := httpClient.R().
		Get(apiRoute(server, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode())
	respBody := map[string]any{}
	json.Unmarshal(resp.Bytes(), &respBody)

	assert.Equal(t, map[string]any{
		"id":         listId,
		"name":       listName,
		"itemsCount": float64(2),
	}, respBody)
}

func apiRoute(server *httptest.Server, listId string) string {
	return server.URL + "/api/lists/" + listId
}
