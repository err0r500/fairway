//go:build test

package userregistered_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/realworldapp/automate"
	"github.com/err0r500/fairway/examples/realworldapp/automate/userregistered"
	"github.com/err0r500/fairway/examples/realworldapp/event"
	"github.com/err0r500/fairway/testing/given"
	"github.com/google/uuid"
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

	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()
	dcbNs := fmt.Sprintf("t-%s", uuid.NewString())

	store := dcb.NewDcbStore(db, dcbNs)

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	emailSender := &InMemoryEmailSender{}

	registry := &automate.AutomationRegistry{}
	userregistered.Register(registry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stopFn, err := registry.StartAll(ctx, store, automate.AllDeps{EmailSender: emailSender})
	require.NoError(t, err)
	defer stopFn()

	// Given/When: UserRegistered event
	given.EventsInStore(store, fairway.NewEvent(event.UserRegistered{
		Id:    "user-1",
		Name:  "johndoe",
		Email: "john@example.com",
	}))

	// Then
	assert.Eventually(t, func() bool {
		sent := emailSender.Sent()
		return len(sent) == 1 && sent[0].Email == "john@example.com" && sent[0].Name == "johndoe"
	}, 2*time.Second, 10*time.Millisecond, "welcome email should be sent")
}
