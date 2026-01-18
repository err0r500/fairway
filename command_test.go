package fairway_test

import (
	"context"
	"encoding/json"
	"iter"
	"reflect"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestBootstrapAppend_NoCondition(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Given - Command that appends without reading (bootstrap scenario)
		store := &mockStore{}
		runner := fairway.NewCommandRunner(store)

		events := RandomEvents(t, 5)

		cmd := &testCommand{
			ShouldRead:     false, // Bootstrap - no read
			EventsToAppend: events,
		}

		// When - Execute the command
		err := runner.RunPure(context.Background(), cmd)
		require.NoError(t, err)

		// Then - Bootstrap append should have nil condition (no optimistic locking)
		assert.Len(t, store.ReadCalls, 0, "bootstrap should not read")
		require.Len(t, store.AppendCalls, 1, "should append once")
		assert.Nil(t, store.AppendCalls[0].Condition, "bootstrap append has no condition")
	})
}

func TestConditionalAppend_AfterRead(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Command that reads events then appends (conditional append)
		storedEvents := RandomStoredEvents(t, 5)
		lastVersionstamp := storedEvents[len(storedEvents)-1].Position

		store := &mockStore{
			ReadEvents: storedEvents,
		}
		runner := fairway.NewCommandRunner(store)

		eventsToAppend := RandomEvents(t, 3)

		cmd := &testCommand{
			ShouldRead:     true,                                            // Conditional append - reads first
			QueryTypes:     []any{TestEventA{}, TestEventB{}, TestEventC{}}, // Register all types
			EventsToAppend: eventsToAppend,
		}

		// When - Execute the command
		err := runner.RunPure(context.Background(), cmd)
		require.NoError(tt, err)

		// Then - Conditional append should have condition with last versionstamp
		require.Len(tt, store.ReadCalls, 1, "should read once")
		require.Len(tt, store.AppendCalls, 1, "should append once")

		condition := store.AppendCalls[0].Condition
		require.NotNil(tt, condition, "append after read should have condition")
		require.NotNil(tt, condition.After, "condition should have versionstamp")
		assert.Equal(tt, lastVersionstamp, *condition.After, "should use last read versionstamp")
		assert.Len(tt, condition.Query.Items, 1, "condition should include query")
	})
}

func TestMultipleReads_LastVersionstampWins(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Command with multiple reads, each read updates the versionstamp
		storedEvents := RandomStoredEvents(t, 5)
		lastVersionstamp := storedEvents[len(storedEvents)-1].Position

		store := &mockStore{
			ReadEvents: storedEvents,
		}

		impl := func(ctx context.Context, ra fairway.EventReadAppender) error {
			// First read
			if err := ra.ReadEvents(ctx,
				fairway.QueryItems(
					fairway.NewQueryItem().Types(TestEventA{}, TestEventB{}, TestEventC{}),
				),
				func(e any, err error) bool {
					return true
				}); err != nil {
				return err
			}

			// Second read (should update versionstamp to last event)
			if err := ra.ReadEvents(ctx,
				fairway.QueryItems(
					fairway.NewQueryItem().Types(TestEventB{}),
				),
				func(e any, err error) bool {
					return true
				}); err != nil {
				return err
			}

			// When - Append after multiple reads
			return ra.AppendEvents(ctx, RandomEvent(t))
		}

		cmdFunc := commandFunc(impl)
		runner := fairway.NewCommandRunner(store)
		err := runner.RunPure(context.Background(), cmdFunc)
		require.NoError(tt, err)

		// Then - Should use last versionstamp from final read
		require.Len(tt, store.AppendCalls, 1)

		condition := store.AppendCalls[0].Condition
		require.NotNil(tt, condition)
		require.NotNil(tt, condition.After)
		assert.Equal(tt, lastVersionstamp, *condition.After, "should use versionstamp from last read")
	})
}

func TestEmptyReadResult_ConditionStillCreated(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Command that reads (finds nothing) then appends
		store := &mockStore{
			ReadEvents: []dcb.StoredEvent{}, // No events
		}
		runner := fairway.NewCommandRunner(store)

		eventsToAppend := RandomEvents(t, 3)

		cmd := &testCommand{
			ShouldRead:     true,
			QueryTypes:     []any{TestEventA{}},
			EventsToAppend: eventsToAppend,
		}

		// When - Execute command
		err := runner.RunPure(context.Background(), cmd)
		require.NoError(tt, err)

		// Then - Condition created even with empty read
		require.Len(tt, store.AppendCalls, 1)

		condition := store.AppendCalls[0].Condition
		require.NotNil(tt, condition, "condition created even with empty read result")
	})
}

func TestHandlerStopsEarly_VersionstampFromLastYielded(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Command that stops reading early
		storedEvents := RandomStoredEventsMin(t, 3, 10)

		stopAfter := rapid.IntRange(1, len(storedEvents)-1).Draw(t, "stopAfter")
		expectedVersionstamp := storedEvents[stopAfter-1].Position

		store := &mockStore{
			ReadEvents: storedEvents,
		}

		impl := func(ctx context.Context, ra fairway.EventReadAppender) error {
			count := 0

			if err := ra.ReadEvents(ctx,
				fairway.QueryItems(
					fairway.NewQueryItem().Types(TestEventA{}, TestEventB{}, TestEventC{}),
				),
				func(e any, err error) bool {
					count++
					return count < stopAfter // Stop early
				}); err != nil {
				return err
			}

			// When - Append after stopped iteration
			return ra.AppendEvents(ctx, RandomEvent(t))
		}

		cmdFunc := commandFunc(impl)
		runner := fairway.NewCommandRunner(store)

		err := runner.RunPure(context.Background(), cmdFunc)
		require.NoError(tt, err)

		// Then - Versionstamp from last yielded event, not last in store
		require.Len(tt, store.AppendCalls, 1)

		condition := store.AppendCalls[0].Condition
		require.NotNil(tt, condition)
		require.NotNil(tt, condition.After)
		assert.Equal(tt, expectedVersionstamp, *condition.After, "should use versionstamp from last yielded event")
	})
}

func TestRoundTripSerialization(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Events with tags to serialize
		store := &mockStore{}
		runner := fairway.NewCommandRunner(store)

		events := RandomEvents(t, 3)
		tags := RandomTags(t)

		tagsList := make([][]string, len(events))
		for i := range events {
			tagsList[i] = tags
		}

		cmd := &testCommand{
			EventsToAppend: events,
			AppendTags:     tagsList,
		}

		// When - Append events
		err := runner.RunPure(context.Background(), cmd)
		require.NoError(tt, err)

		// Then - Events serialized correctly
		require.Len(tt, store.AppendCalls, 1)

		for i, dcbEvent := range store.AppendCalls[0].Events {
			assert.NotEmpty(tt, dcbEvent.Type, "event %d should have type", i)
			assert.Equal(tt, tags, dcbEvent.Tags, "event %d tags should match", i)
			assert.NotEmpty(tt, dcbEvent.Data, "event %d should have data", i)
		}
	})
}

func TestCustomTyper(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		EventsToAppend: []any{CustomTypedEvent{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, store.AppendCalls, 1)

	dcbEvent := store.AppendCalls[0].Events[0]
	assert.Equal(t, "custom-type-name", dcbEvent.Type)
}

func TestMultipleEventsPreserveOrder(tt *testing.T) {
	rapid.Check(tt, func(t *rapid.T) {
		// Given - Multiple events with different tags
		store := &mockStore{}
		runner := fairway.NewCommandRunner(store)

		events := RandomEvents(t, 10)
		tagsList := make([][]string, len(events))
		for i := range events {
			tagsList[i] = RandomTags(t)
		}

		cmd := &testCommand{
			EventsToAppend: events,
			AppendTags:     tagsList,
		}

		// When - Append multiple events
		err := runner.RunPure(context.Background(), cmd)
		require.NoError(tt, err)

		// Then - Order preserved
		require.Len(tt, store.AppendCalls, 1)

		appendedEvents := store.AppendCalls[0].Events
		require.Len(tt, appendedEvents, len(events), "all events appended")

		// Verify tags match in order
		for i, evt := range appendedEvents {
			assert.Equal(tt, tagsList[i], evt.Tags, "event %d tags should match", i)
		}
	})
}

func TestRunPure_ExecutesCommand(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	executed := false
	cmd := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		executed = true
		return nil
	})

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	assert.True(t, executed)
}

func TestRunPure_PropagatesCommandErrors(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	expectedErr := dcb.ErrInvalidQuery
	cmd := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		return expectedErr
	})

	err := runner.RunPure(context.Background(), cmd)
	assert.Equal(t, expectedErr, err)
}

func TestRunPure_PropagatesReadErrors(t *testing.T) {
	expectedErr := dcb.ErrAppendConditionFailed
	store := &mockStore{
		ReadError: expectedErr,
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(context.Background(), cmd)
	assert.Equal(t, expectedErr, err)
}

func TestRunPure_PropagatesAppendErrors(t *testing.T) {
	expectedErr := dcb.ErrAppendConditionFailed
	store := &mockStore{
		AppendError: expectedErr,
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	assert.Equal(t, expectedErr, err)
}

func TestRunWithEffect_PassesDependencies(t *testing.T) {
	type Deps struct {
		Value string
	}

	type EffectCommand struct {
		T            *testing.T
		ReceivedDeps *Deps
	}

	impl := func(cmd *EffectCommand) fairway.CommandWithEffect[Deps] {
		return commandWithEffectFunc[Deps](func(ctx context.Context, ra fairway.EventReadAppender, deps Deps) error {
			cmd.ReceivedDeps = &deps
			return ra.AppendEvents(ctx, TestEventA{Value: deps.Value})
		})
	}

	store := &mockStore{}
	deps := Deps{Value: "test-dep"}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	cmd := &EffectCommand{T: t}
	err := runner.RunWithEffect(context.Background(), impl(cmd))
	require.NoError(t, err)

	require.NotNil(t, cmd.ReceivedDeps, "expected deps to be passed")

	assert.Equal(t, "test-dep", cmd.ReceivedDeps.Value)
}

func TestRunPure_WorksOnCommandWithEffectRunner(t *testing.T) {
	type Deps struct {
		Value string
	}

	store := &mockStore{}
	deps := Deps{Value: "unused"}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	// Pure command should work
	cmd := &testCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "pure"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, store.AppendCalls, 1)
}

func TestDifferentDependencyTypes(t *testing.T) {
	type LoggerDeps struct {
		Name string
	}

	type DatabaseDeps struct {
		ConnString string
	}

	// Logger runner
	store1 := &mockStore{}
	loggerRunner := fairway.NewCommandWithEffectRunner(store1, LoggerDeps{Name: "logger"})

	loggerCmd := commandWithEffectFunc[LoggerDeps](func(ctx context.Context, ra fairway.EventReadAppender, deps LoggerDeps) error {
		assert.Equal(t, "logger", deps.Name)
		return nil
	})

	err := loggerRunner.RunWithEffect(context.Background(), loggerCmd)
	require.NoError(t, err, "logger command failed")

	// Database runner
	store2 := &mockStore{}
	dbRunner := fairway.NewCommandWithEffectRunner(store2, DatabaseDeps{ConnString: "db://host"})

	dbCmd := commandWithEffectFunc[DatabaseDeps](func(ctx context.Context, ra fairway.EventReadAppender, deps DatabaseDeps) error {
		assert.Equal(t, "db://host", deps.ConnString)
		return nil
	})

	err = dbRunner.RunWithEffect(context.Background(), dbCmd)
	require.NoError(t, err, "database command failed")
}

func TestCancelledContextDuringRead(t *testing.T) {
	store := &mockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"test"}`)}, Position: dcb.Versionstamp{}},
		},
	}
	runner := fairway.NewCommandRunner(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := &testCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(ctx, cmd)
	require.Error(t, err, "expected error from cancelled context")

	assert.Equal(t, context.Canceled, err)
}

func TestCancelledContextDuringAppend(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := &testCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(ctx, cmd)
	require.Error(t, err, "expected error from cancelled context")

	assert.Equal(t, context.Canceled, err)
}

func TestTypeOnlyQuery(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}, TestEventB{}},
		EventsToAppend: []any{TestEventC{Flag: true}},
	}

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, store.ReadCalls, 1)

	query := store.ReadCalls[0].Query
	require.Len(t, query.Items, 1)

	item := query.Items[0]
	assert.Len(t, item.Types, 2)
	assert.Len(t, item.Tags, 0)
}

func TestTagOnlyQuery(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		ShouldRead:     true,
		QueryTags:      []string{"tag1", "tag2"},
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, store.ReadCalls, 1)

	query := store.ReadCalls[0].Query
	require.Len(t, query.Items, 1)

	item := query.Items[0]
	assert.Len(t, item.Types, 0)
	assert.Len(t, item.Tags, 2)
}

func TestTypesAndTagsQuery(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &testCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		QueryTags:      []string{"tag1"},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(context.Background(), cmd)
	require.NoError(t, err)

	require.Len(t, store.ReadCalls, 1)

	query := store.ReadCalls[0].Query
	require.Len(t, query.Items, 1)

	item := query.Items[0]
	assert.Len(t, item.Types, 1)
	assert.Len(t, item.Tags, 1)
}

func TestMultipleQueryItems_OR(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	// Build command manually to have multiple query items
	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		if err := ra.ReadEvents(ctx,
			fairway.QueryItems(
				fairway.NewQueryItem().Types(TestEventA{}),
				fairway.NewQueryItem().Types(TestEventB{}),
			),
			func(e any, err error) bool {
				return true
			}); err != nil {
			return err
		}

		return ra.AppendEvents(ctx, TestEventC{Flag: true})
	})

	err := runner.RunPure(context.Background(), impl)
	require.NoError(t, err)

	require.Len(t, store.ReadCalls, 1)

	query := store.ReadCalls[0].Query
	assert.Len(t, query.Items, 2, "expected 2 query items (OR)")
}

func TestEmptyAppend_Ignored(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		return ra.AppendEvents(ctx) // Empty append
	})

	err := runner.RunPure(context.Background(), impl)
	require.NoError(t, err)

	// No append call should be recorded
	assert.Len(t, store.AppendCalls, 0)
}

func TestReadEvents_NilHandler(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		// Handler with nil Handle function
		return ra.ReadEvents(ctx, fairway.Query{}, nil)
	})

	err := runner.RunPure(context.Background(), impl)
	require.NoError(t, err)
}

func TestMultipleAppends_InOneCommand(t *testing.T) {
	store := &mockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		if err := ra.AppendEvents(ctx, TestEventA{Value: "first"}); err != nil {
			return err
		}
		return ra.AppendEvents(ctx, TestEventB{Count: 2})
	})

	err := runner.RunPure(context.Background(), impl)
	require.NoError(t, err)

	// Both appends should be captured
	require.Len(t, store.AppendCalls, 2)

	// Both should have same condition (nil in this case, no read)
	assert.Nil(t, store.AppendCalls[0].Condition)
	assert.Nil(t, store.AppendCalls[1].Condition)
}

func TestReadAppendReadAppend(t *testing.T) {
	vs1 := dcb.Versionstamp{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	vs2 := dcb.Versionstamp{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	store := &mockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"a"}`)}, Position: vs1},
			{Event: dcb.Event{Type: "TestEventB", Data: []byte(`{"Count":1}`)}, Position: vs2},
		},
	}

	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		// First read - register both types to handle all events from mock
		if err := ra.ReadEvents(ctx,
			fairway.QueryItems(
				fairway.NewQueryItem().Types(TestEventA{}, TestEventB{}),
			),
			func(e any, err error) bool {
				return true
			}); err != nil {
			return err
		}

		// First append
		if err := ra.AppendEvents(ctx, TestEventB{Count: 100}); err != nil {
			return err
		}

		// Second read (should update versionstamp)
		if err := ra.ReadEvents(ctx,
			fairway.QueryItems(
				fairway.NewQueryItem().Types(TestEventB{}),
			),
			func(e any, err error) bool {
				return true
			}); err != nil {
			return err
		}

		// Second append
		return ra.AppendEvents(ctx, TestEventC{Flag: true})
	})

	err := runner.RunPure(context.Background(), impl)
	require.NoError(t, err)

	require.Len(t, store.AppendCalls, 2)

	// First append should use vs2 from first read (last event seen)
	cond1 := store.AppendCalls[0].Condition
	require.NotNil(t, cond1, "expected first append to have condition with versionstamp")
	require.NotNil(t, cond1.After, "expected first append to have condition with versionstamp")
	assert.Equal(t, vs2, *cond1.After)

	// Second append should also use vs2 from second read (both events returned by mock)
	cond2 := store.AppendCalls[1].Condition
	require.NotNil(t, cond2, "expected second append to have condition with versionstamp")
	require.NotNil(t, cond2.After, "expected second append to have condition with versionstamp")
	assert.Equal(t, vs2, *cond2.After)
}

// mockStore provides controllable DcbStore for testing
type mockStore struct {
	// What to return from Read()
	ReadEvents []dcb.StoredEvent
	ReadError  error

	// What to return from Append()
	AppendError error

	// Captured calls (for assertions)
	AppendCalls []appendCall
	ReadCalls   []readCall
}

type appendCall struct {
	Events    []dcb.Event
	Condition *dcb.AppendCondition
}

type readCall struct {
	Query dcb.Query
	Opts  *dcb.ReadOptions
}

func (m *mockStore) Read(ctx context.Context, query dcb.Query, opts *dcb.ReadOptions) iter.Seq2[dcb.StoredEvent, error] {
	m.ReadCalls = append(m.ReadCalls, readCall{Query: query, Opts: opts})
	return func(yield func(dcb.StoredEvent, error) bool) {
		if m.ReadError != nil {
			yield(dcb.StoredEvent{}, m.ReadError)
			return
		}
		for _, evt := range m.ReadEvents {
			if ctx.Err() != nil {
				yield(dcb.StoredEvent{}, ctx.Err())
				return
			}
			if !yield(evt, nil) {
				return
			}
		}
	}
}

func (m *mockStore) Append(ctx context.Context, events []dcb.Event, condition *dcb.AppendCondition) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.AppendCalls = append(m.AppendCalls, appendCall{Events: events, Condition: condition})
	return m.AppendError
}

func (m *mockStore) ReadAll(ctx context.Context) iter.Seq2[dcb.StoredEvent, error] {
	panic("ReadAll not implemented in mock")
}

// testCommand provides hooks for observing command execution
type testCommand struct {
	T *testing.T

	// Behavior configuration
	ShouldRead     bool
	QueryTypes     []any
	QueryTags      []string
	EventsToAppend []fairway.Event
	AppendTags     [][]string

	// Observation hooks
	OnRead         func(e any)
	OnBeforeAppend func()
	OnAfterAppend  func(err error)

	// Captured state
	ReadCount       int
	AppendAttempted bool
}

func (cmd *testCommand) Run(ctx context.Context, ra fairway.EventReadAppender) error {
	if cmd.ShouldRead {
		// Build query items
		queryItems := make([]fairway.QueryItem, 0)
		if len(cmd.QueryTypes) > 0 || len(cmd.QueryTags) > 0 {
			qi := fairway.NewQueryItem()
			if len(cmd.QueryTypes) > 0 {
				qi = qi.Types(cmd.QueryTypes...)
			}
			if len(cmd.QueryTags) > 0 {
				qi = qi.Tags(cmd.QueryTags...)
			}
			queryItems = append(queryItems, qi)
		}

		query := fairway.QueryItems(queryItems...)
		handler := func(e any, err error) bool {
			if err != nil {
				return false
			}
			cmd.ReadCount++
			if cmd.OnRead != nil {
				cmd.OnRead(e)
			}
			return true
		}

		if err := ra.ReadEvents(ctx, query, handler); err != nil {
			return err
		}
	}

	if cmd.OnBeforeAppend != nil {
		cmd.OnBeforeAppend()
	}

	cmd.AppendAttempted = true

	// Build events
	events := make([]fairway.Event, len(cmd.EventsToAppend))
	for i, evt := range cmd.EventsToAppend {
		if i < len(cmd.AppendTags) && len(cmd.AppendTags[i]) > 0 {
			// Wrap event with tags
			events[i] = &testEventWithTags{event: evt, tags: cmd.AppendTags[i]}
		} else {
			events[i] = evt
		}
	}

	err := ra.AppendEvents(ctx, events...)

	if cmd.OnAfterAppend != nil {
		cmd.OnAfterAppend(err)
	}

	return err
}

// Event types for testing
type TestEventA struct {
	Value string
}

func (TestEventA) Tags() []string { return nil }

type TestEventB struct {
	Count int
}

func (TestEventB) Tags() []string { return nil }

type TestEventC struct {
	Flag bool
}

func (TestEventC) Tags() []string { return nil }

// testEventWithTags wraps an event with tags for testing
type testEventWithTags struct {
	event fairway.Event
	tags  []string
}

func (e *testEventWithTags) Tags() []string {
	return e.tags
}

// MarshalJSON marshals the inner event, not the wrapper
func (e *testEventWithTags) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.event)
}

// TypeString returns the type name of the inner event
func (e *testEventWithTags) TypeString() string {
	// Check if inner event implements Typer
	if typer, ok := e.event.(fairway.Typer); ok {
		return typer.TypeString()
	}
	// Otherwise use reflection on inner event
	return reflect.TypeOf(e.event).Name()
}

// Custom Typer for testing
type CustomTypedEvent struct {
	Value string
}

func (CustomTypedEvent) TypeString() string {
	return "custom-type-name"
}

func (CustomTypedEvent) Tags() []string { return nil }

// Helper: command from function
type commandFunc func(context.Context, fairway.EventReadAppender) error

func (f commandFunc) Run(ctx context.Context, ra fairway.EventReadAppender) error {
	return f(ctx, ra)
}

// Helper: CommandWithEffect from function
type commandWithEffectFunc[Deps any] func(context.Context, fairway.EventReadAppender, Deps) error

func (f commandWithEffectFunc[Deps]) Run(ctx context.Context, ra fairway.EventReadAppender, deps Deps) error {
	return f(ctx, ra, deps)
}
