package additem_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/err0r500/fairway/examples/todolist/change/additem"
	"github.com/err0r500/fairway/examples/todolist/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/testing/then"
	"github.com/stretchr/testify/assert"
)

func TestCreateItem_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, additem.Register)

	// Given
	itemId := "item-1"
	listId := "list-1"
	itemText := "item text"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"Text": itemText}).
		Post(apiRoute(server, itemId, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode())
	assert.Empty(t, string(resp.Bytes()))
	then.ExpectEventsInStore(t, store, event.ItemCreated{Id: itemId, ListId: listId, Text: itemText})
}

func TestCreateItem_Conflict(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, additem.Register)

	// Given
	itemId := "item-1"
	listId := "list-1"
	initialEvent := event.ItemCreated{Id: itemId, ListId: "list 1 name", Text: ""}
	given.EventsInStore(store, initialEvent)

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{"text": "anything"}).
		Post(apiRoute(server, itemId, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
	then.ExpectEventsInStore(t, store, initialEvent)
}

func TestCreateItem_ApiValidation(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, additem.Register)

	// Given
	itemId := "item-1"
	listId := "list-1"

	// When
	resp, err := httpClient.R().
		SetBody(map[string]any{}).
		Post(apiRoute(server, itemId, listId))

	// Then
	assert.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode())
	then.ExpectEventsInStore(t, store)
}

func apiRoute(server *httptest.Server, itemId, listId string) string {
	return server.URL + "/api/lists/" + listId + "/items/" + itemId
}
