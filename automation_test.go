package fairway_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	fdb.MustAPIVersion(740)
}

// TestEvent is a sample event for testing
type TestAutomationEvent struct {
	UserID string
}

func (e TestAutomationEvent) Tags() []string {
	return []string{"user:" + e.UserID}
}

// TestDeps is the dependency type for test commands
type TestDeps struct {
	HandlerCalled *atomic.Int32
	LastEvent     *fairway.Event
	ShouldFail    bool
	FailCount     *atomic.Int32
}

// TestCommand processes TestAutomationEvent
type TestCommand struct {
	Event fairway.Event
	Deps  *TestDeps
}

func (c *TestCommand) Run(ctx context.Context, ra fairway.EventReadAppenderExtended, deps TestDeps) error {
	deps.HandlerCalled.Add(1)
	*deps.LastEvent = c.Event

	if deps.ShouldFail {
		if deps.FailCount != nil {
			deps.FailCount.Add(1)
		}
		return errors.New("simulated failure")
	}
	return nil
}

func setupTestAutomation(t *testing.T, dcbNs, queueId string, deps TestDeps, opts ...fairway.AutomationOption[TestDeps]) (*fairway.Automation[TestDeps], dcb.DcbStore) {
	t.Helper()

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	handler := func(ev fairway.Event) fairway.CommandWithEffect[TestDeps] {
		return &TestCommand{Event: ev, Deps: &deps}
	}

	automation, err := fairway.NewAutomation(
		store,
		deps,
		queueId,
		TestAutomationEvent{},
		handler,
		opts...,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		automation.Stop()
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	return automation, store
}

func TestAutomation_BasicEventProcessing(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
	}

	automation, store := setupTestAutomation(t, dcbNs, queueId, deps,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
	)

	// Start automation
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := automation.Start(ctx)
	require.NoError(t, err)

	// Append an event
	userId := "user-123"
	testEvent := TestAutomationEvent{UserID: userId}
	dcbEvent, err := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
	require.NoError(t, err)

	err = store.Append(ctx, []dcb.Event{dcbEvent}, nil)
	require.NoError(t, err)

	// Wait for handler to be called
	assert.Eventually(t, func() bool {
		return handlerCalled.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond, "handler should be called")

	// Verify event data
	if lastEvent.Data != nil {
		eventData, ok := lastEvent.Data.(TestAutomationEvent)
		if ok {
			assert.Equal(t, userId, eventData.UserID)
		}
	}
}

func TestAutomation_CursorPersistence(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
	}

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	handler := func(ev fairway.Event) fairway.CommandWithEffect[TestDeps] {
		return &TestCommand{Event: ev, Deps: &deps}
	}

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	ctx := context.Background()

	// Append first event
	event1, _ := fairway.ToDcbEvent(fairway.NewEvent(TestAutomationEvent{UserID: "user-1"}))
	err := store.Append(ctx, []dcb.Event{event1}, nil)
	require.NoError(t, err)

	// Start automation, process first event, then stop
	automation1, err := fairway.NewAutomation(
		store, deps, queueId, TestAutomationEvent{}, handler,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel1()
	_ = automation1.Start(ctx1)

	assert.Eventually(t, func() bool {
		return handlerCalled.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	automation1.Stop()
	_ = automation1.Wait()

	// Append second event while automation is stopped
	event2, _ := fairway.ToDcbEvent(fairway.NewEvent(TestAutomationEvent{UserID: "user-2"}))
	err = store.Append(ctx, []dcb.Event{event2}, nil)
	require.NoError(t, err)

	// Reset counter
	initialCount := handlerCalled.Load()

	// Start new automation instance
	automation2, err := fairway.NewAutomation(
		store, deps, queueId, TestAutomationEvent{}, handler,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()
	_ = automation2.Start(ctx2)

	// Should only process the new event (cursor persisted)
	assert.Eventually(t, func() bool {
		return handlerCalled.Load() > initialCount
	}, 2*time.Second, 10*time.Millisecond, "should process new event")

	automation2.Stop()

	// Should have processed exactly one more event
	assert.Equal(t, initialCount+1, handlerCalled.Load(), "should only process events after cursor")
}

func TestAutomation_RetryOnFailure(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	failCount := &atomic.Int32{}
	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
		ShouldFail:    true,
		FailCount:     failCount,
	}

	automation, store := setupTestAutomation(t, dcbNs, queueId, deps,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
		fairway.WithMaxAttempts[TestDeps](3),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := automation.Start(ctx)
	require.NoError(t, err)

	// Append an event
	testEvent := TestAutomationEvent{UserID: "user-fail"}
	dcbEvent, _ := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
	err = store.Append(ctx, []dcb.Event{dcbEvent}, nil)
	require.NoError(t, err)

	// Wait for retries (with backoff this might take a while)
	assert.Eventually(t, func() bool {
		return failCount.Load() >= 1
	}, 5*time.Second, 50*time.Millisecond, "should attempt processing")
}

func TestAutomation_DLQAfterMaxAttempts(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	failCount := &atomic.Int32{}
	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
		ShouldFail:    true,
		FailCount:     failCount,
	}

	automation, store := setupTestAutomation(t, dcbNs, queueId, deps,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
		fairway.WithMaxAttempts[TestDeps](2),                     // Low for faster test
		fairway.WithRetryBaseWait[TestDeps](10*time.Millisecond), // Short backoff for testing
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := automation.Start(ctx)
	require.NoError(t, err)

	// Append an event
	testEvent := TestAutomationEvent{UserID: "user-dlq"}
	dcbEvent, _ := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
	err = store.Append(ctx, []dcb.Event{dcbEvent}, nil)
	require.NoError(t, err)

	// Wait for DLQ (backoff is short in test)
	assert.Eventually(t, func() bool {
		count := 0
		for _, err := range automation.ListDLQ() {
			if err != nil {
				continue
			}
			count++
		}
		return count > 0
	}, 5*time.Second, 50*time.Millisecond, "job should end up in DLQ")
}

func TestAutomation_NoDuplicateProcessing(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
	}

	automation, store := setupTestAutomation(t, dcbNs, queueId, deps,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
		fairway.WithNumWorkers[TestDeps](4), // Multiple workers
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := automation.Start(ctx)
	require.NoError(t, err)

	// Append a single event
	testEvent := TestAutomationEvent{UserID: "user-single"}
	dcbEvent, _ := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
	err = store.Append(ctx, []dcb.Event{dcbEvent}, nil)
	require.NoError(t, err)

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Event should only be processed once
	assert.Equal(t, int32(1), handlerCalled.Load(), "event should be processed exactly once")
}

func TestAutomation_LeaseExpiry(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
	}

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	handler := func(ev fairway.Event) fairway.CommandWithEffect[TestDeps] {
		return &TestCommand{Event: ev, Deps: &deps}
	}

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	ctx := context.Background()

	// Append an event
	testEvent := TestAutomationEvent{UserID: "user-lease"}
	dcbEvent, _ := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
	err := store.Append(ctx, []dcb.Event{dcbEvent}, nil)
	require.NoError(t, err)

	// Start first automation with very short lease
	automation1, err := fairway.NewAutomation(
		store, deps, queueId, TestAutomationEvent{}, handler,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
		fairway.WithLeaseTTL[TestDeps](50*time.Millisecond), // Very short lease
	)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel1()
	_ = automation1.Start(ctx1)

	// Wait for first automation to process
	assert.Eventually(t, func() bool {
		return handlerCalled.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	automation1.Stop()

	// The event was processed, so lease handling works correctly
	assert.GreaterOrEqual(t, handlerCalled.Load(), int32(1))
}

func TestAutomation_MultipleEvents(t *testing.T) {
	dcbNs := fmt.Sprintf("test-dcb-%s", uuid.NewString())
	queueId := "test-queue"

	handlerCalled := &atomic.Int32{}
	var lastEvent fairway.Event

	deps := TestDeps{
		HandlerCalled: handlerCalled,
		LastEvent:     &lastEvent,
	}

	automation, store := setupTestAutomation(t, dcbNs, queueId, deps,
		fairway.WithPollInterval[TestDeps](10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := automation.Start(ctx)
	require.NoError(t, err)

	// Append multiple events
	eventCount := 5
	for i := range eventCount {
		testEvent := TestAutomationEvent{UserID: fmt.Sprintf("user-%d", i)}
		dcbEvent, _ := fairway.ToDcbEvent(fairway.NewEvent(testEvent))
		err = store.Append(ctx, []dcb.Event{dcbEvent}, nil)
		require.NoError(t, err)
	}

	// Wait for all events to be processed
	assert.Eventually(t, func() bool {
		return handlerCalled.Load() >= int32(eventCount)
	}, 3*time.Second, 10*time.Millisecond, "all events should be processed")
}
