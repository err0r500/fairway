//go:build test

package createlist_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/todolist/core/change/createlist"
	"github.com/err0r500/fairway/examples/todolist/core/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateList_Success(t *testing.T) {
	t.Parallel()
	// Given - HTTP server with command handler
	store, server := setup(t)
	defer server.Close()

	// When - POST to create list
	body := `{"name":"My List"}`
	resp, err := http.Post(server.URL+"/api/lists/list-1", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Then - 201 Created
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	// And - ListCreated event in store
	events := dcb.CollectEvents(t, store.ReadAll(t.Context()))

	require.Len(t, events, 1)

	var evt event.ListCreated
	err = json.Unmarshal(events[0].Event.Data, &evt)
	require.NoError(t, err)
	assert.Equal(t, "list-1", evt.ListId)
	assert.Equal(t, "My List", evt.Name)
}

func TestCreateList_Conflict(t *testing.T) {
	t.Parallel()
	// Given - store with existing ListCreated event
	store, server := setup(t)
	defer server.Close()

	listId := "list-1"
	err := given.EventsInStore(store,
		event.ListCreated{ListId: listId, Name: "Existing"},
	)
	require.NoError(t, err)

	// When - POST to create duplicate
	body := `{"name":"Another List"}`
	resp, err := http.Post(server.URL+"/api/lists/"+listId, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	// Then - 409 Conflict
	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	// And - no new event (still 1)
	events := dcb.CollectEvents(t, store.Read(t.Context(),
		dcb.Query{Items: []dcb.QueryItem{{
			Types: []string{"ListCreated"},
			Tags:  []string{event.TagListId(listId)},
		}}}, nil))
	assert.Len(t, events, 1)
}

func setup(t *testing.T) (dcb.DcbStore, *httptest.Server) {
	store := dcb.SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)

	registry := &fairway.HttpChangeRegistry{}
	createlist.Register(registry)

	mux := http.NewServeMux()
	registry.RegisterRoutes(mux, runner)

	server := httptest.NewServer(mux)
	return store, server
}
