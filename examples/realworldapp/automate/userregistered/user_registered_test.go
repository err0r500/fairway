package userregistered_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/automate"
	"github.com/err0r500/fairway/examples/realworldapp/automate/userregistered"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/testing/then"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type InMemoryEmailSender struct {
	mu     sync.Mutex
	emails []SentEmail
}

type SentEmail struct {
	Email string
	Name  string
}

func (s *InMemoryEmailSender) SendWelcomeEmail(ctx context.Context, email, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emails = append(s.emails, SentEmail{Email: email, Name: name})
	return nil
}

func (s *InMemoryEmailSender) Sent() []SentEmail {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SentEmail{}, s.emails...)
}

func TestUserRegistered_SendsWelcomeEmail(t *testing.T) {
	t.Parallel()

	store := given.SetupTestStore(t)
	emailSender := &InMemoryEmailSender{}
	registry := &fairway.AutomationRegistry[automate.AllDeps]{}
	userregistered.Register(registry)
	stopFn, err := registry.StartAll(t.Context(), store, automate.AllDeps{EmailSender: emailSender})
	require.NoError(t, err)
	defer stopFn()

	// Given/When: UserRegistered event
	initialEvent := event.UserRegistered{
		Id:    "user-1",
		Name:  "johndoe",
		Email: "john@example.com",
	}
	given.EventsInStore(store, fairway.NewEvent(initialEvent))

	// when
	waitForEventCountInStore(t, store, 2)

	// Then
	assert.Equal(t, emailSender.Sent(), []SentEmail{{Email: initialEvent.Email, Name: initialEvent.Name}})
	then.ExpectEventsInStore(t, store, fairway.NewEvent(initialEvent), fairway.NewEvent(event.UserWelcomeEmailSent{UserId: initialEvent.Id}))
}

func waitForEventCountInStore(t *testing.T, store dcb.DcbStore, count int) {
	assert.Eventually(t, func() bool {
		eventsCount := 0
		for range store.ReadAll(t.Context()) {
			eventsCount++
		}
		return eventsCount == count
	}, 2*time.Second, 10*time.Millisecond, "events should be in store")

}
