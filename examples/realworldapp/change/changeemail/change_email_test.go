package changeemail_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/examples/realworldapp/change/changeemail"
	"github.com/err0r500/fairway/examples/realworldapp/crypto"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/stretchr/testify/assert"
)

func TestChangeEmail_Success(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeemail.Register)
	currUserId := "user-1"
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}))

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": "newemail@example.com",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeEmail_CannotTakeTakenEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeemail.Register)
	currUserId := "user-1"
	takenEmail := "taken@example.com"
	given.EventsInStore(store,
		fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserRegistered{Id: "user-2", Name: "other", Email: takenEmail, HashedPassword: "h"}),
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": takenEmail,
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeEmail_CannotTakeRecentlyReleasedEmail(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeemail.Register)
	currUserId := "user-1"
	recentlyReleasedEmail := "recent@example.com"

	// other user released email 1 day ago (less than 3 days)
	oneDayAgo := time.Now().Add(-24 * time.Hour)
	given.EventsInStore(store,
		fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserRegistered{Id: "user-2", Name: "other", Email: recentlyReleasedEmail, HashedPassword: "h"}),
		fairway.NewEventAt(event.UserChangedTheirEmail{UserId: "user-2", PreviousEmail: recentlyReleasedEmail, NewEmail: "new@example.com"}, oneDayAgo),
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": recentlyReleasedEmail,
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusConflict, resp.StatusCode())
}

func TestChangeEmail_CanTakeEmailReleasedOver3DaysAgo(t *testing.T) {
	t.Parallel()
	store, server, httpClient := given.FreshSetup(t, changeemail.Register)
	currUserId := "user-1"
	oldEmail := "old@example.com"

	// other user released email 4 days ago (more than 3 days)
	fourDaysAgo := time.Now().Add(-4 * 24 * time.Hour)
	given.EventsInStore(store,
		fairway.NewEvent(event.UserRegistered{Id: currUserId, Name: "john", Email: "john@example.com", HashedPassword: "h"}),
		fairway.NewEvent(event.UserRegistered{Id: "user-2", Name: "other", Email: oldEmail, HashedPassword: "h"}),
		fairway.NewEventAt(event.UserChangedTheirEmail{UserId: "user-2", PreviousEmail: oldEmail, NewEmail: "new@example.com"}, fourDaysAgo),
	)

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, currUserId)).
		SetBody(map[string]any{
			"email": oldEmail,
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode())
}

func TestChangeEmail_UnauthenticatedFails(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, changeemail.Register)

	resp, err := httpClient.R().
		SetBody(map[string]any{
			"email": "john@example.com",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode())
}

func TestChangeEmail_UserNotFound(t *testing.T) {
	t.Parallel()
	_, server, httpClient := given.FreshSetup(t, changeemail.Register)
	unknownUserId := "unknown-user"

	resp, err := httpClient.R().
		SetHeader("Authorization", "Token "+generateToken(t, unknownUserId)).
		SetBody(map[string]any{
			"email": "newemail@example.com",
		}).
		Put(apiRoute(server))

	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode())
}

func generateToken(t *testing.T, userID string) string {
	token, err := crypto.JwtService.Token(userID)
	assert.NoError(t, err)
	return token
}

func apiRoute(server *httptest.Server) string {
	return server.URL + "/user/email"
}
