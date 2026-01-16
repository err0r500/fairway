package fairway_test

import (
	"context"
	"iter"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
)

// MockStore provides controllable DcbStore for testing
type MockStore struct {
	// What to return from Read()
	ReadEvents []dcb.StoredEvent
	ReadError  error

	// What to return from Append()
	AppendError error

	// Captured calls (for assertions)
	AppendCalls []AppendCall
	ReadCalls   []ReadCall
}

type AppendCall struct {
	Events    []dcb.Event
	Condition *dcb.AppendCondition
}

type ReadCall struct {
	Query dcb.Query
	Opts  *dcb.ReadOptions
}

func (m *MockStore) Read(ctx context.Context, query dcb.Query, opts *dcb.ReadOptions) iter.Seq2[dcb.StoredEvent, error] {
	m.ReadCalls = append(m.ReadCalls, ReadCall{Query: query, Opts: opts})
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

func (m *MockStore) Append(ctx context.Context, events []dcb.Event, condition *dcb.AppendCondition) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	m.AppendCalls = append(m.AppendCalls, AppendCall{Events: events, Condition: condition})
	return m.AppendError
}

func (m *MockStore) ReadAll(ctx context.Context) iter.Seq2[dcb.StoredEvent, error] {
	panic("ReadAll not implemented in mock")
}

// TestCommand provides hooks for observing command execution
type TestCommand struct {
	T *testing.T

	// Behavior configuration
	ShouldRead     bool
	QueryTypes     []any
	QueryTags      []string
	EventsToAppend []any
	AppendTags     [][]string

	// Observation hooks
	OnRead         func(te fairway.TaggedEvent)
	OnBeforeAppend func()
	OnAfterAppend  func(err error)

	// Captured state
	ReadCount       int
	AppendAttempted bool
}

func (cmd *TestCommand) Run(ctx context.Context, ra fairway.EventReadAppender) error {
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

		handler := fairway.QueryItems(queryItems...).Handle(func(te fairway.TaggedEvent, err error) bool {
			if err != nil {
				return false
			}
			cmd.ReadCount++
			if cmd.OnRead != nil {
				cmd.OnRead(te)
			}
			return true
		})

		if err := ra.ReadEvents(ctx, handler); err != nil {
			return err
		}
	}

	if cmd.OnBeforeAppend != nil {
		cmd.OnBeforeAppend()
	}

	cmd.AppendAttempted = true

	// Build events
	events := make([]fairway.TaggedEvent, len(cmd.EventsToAppend))
	for i, evt := range cmd.EventsToAppend {
		tags := []string{}
		if i < len(cmd.AppendTags) {
			tags = cmd.AppendTags[i]
		}
		events[i] = fairway.Event(evt, tags...)
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

type TestEventB struct {
	Count int
}

type TestEventC struct {
	Flag bool
}

// Custom Typer for testing
type CustomTypedEvent struct {
	Value string
}

func (CustomTypedEvent) TypeString() string {
	return "custom-type-name"
}

// Test 1: Bootstrap append - no condition
func TestBootstrapAppend_NoCondition(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     false, // Bootstrap - no read
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify no read calls
	if len(store.ReadCalls) != 0 {
		t.Errorf("expected 0 read calls, got %d", len(store.ReadCalls))
	}

	// Verify append call with nil condition
	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	if store.AppendCalls[0].Condition != nil {
		t.Errorf("expected nil condition for bootstrap append, got %+v", store.AppendCalls[0].Condition)
	}
}

// Test 2: Conditional append after read
func TestConditionalAppend_AfterRead(t *testing.T) {
	vs := dcb.Versionstamp{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 0, 1}
	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{
			{
				Event: dcb.Event{
					Type: "TestEventA",
					Tags: []string{"tag1"},
					Data: []byte(`{"Value":"existing"}`),
				},
				Position: vs,
			},
		},
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		QueryTags:      []string{"tag1"},
		EventsToAppend: []any{TestEventB{Count: 42}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify read was called
	if len(store.ReadCalls) != 1 {
		t.Fatalf("expected 1 read call, got %d", len(store.ReadCalls))
	}

	// Verify append has condition with query and versionstamp
	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	condition := store.AppendCalls[0].Condition
	if condition == nil {
		t.Fatal("expected non-nil condition after read")
	}

	if condition.After == nil {
		t.Error("expected After versionstamp to be set")
	} else if *condition.After != vs {
		t.Errorf("expected After versionstamp %v, got %v", vs, *condition.After)
	}

	// Verify query structure
	if len(condition.Query.Items) != 1 {
		t.Errorf("expected 1 query item, got %d", len(condition.Query.Items))
	}
}

// Test 3: Multiple reads - last versionstamp wins
func TestMultipleReads_LastVersionstampWins(t *testing.T) {
	vs1 := dcb.Versionstamp{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	vs2 := dcb.Versionstamp{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	impl := func(ctx context.Context, ra fairway.EventReadAppender) error {
		// First read
		handler1 := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventA{}, TestEventB{}), // Register both types
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			return true
		})
		if err := ra.ReadEvents(ctx, handler1); err != nil {
			return err
		}

		// Second read (different query, will get new versionstamp)
		handler2 := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventB{}),
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			return true
		})
		if err := ra.ReadEvents(ctx, handler2); err != nil {
			return err
		}

		// Append
		return ra.AppendEvents(ctx, fairway.Event(TestEventC{Flag: true}))
	}

	// Wrap as command
	cmdFunc := commandFunc(impl)

	// Both reads will see both events (mock returns all configured events)
	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"a"}`)}, Position: vs1},
			{Event: dcb.Event{Type: "TestEventB", Data: []byte(`{"Count":1}`)}, Position: vs2},
		},
	}

	runner := fairway.NewCommandRunner(store)
	err := runner.RunPure(context.Background(), cmdFunc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// The second read should overwrite the versionstamp
	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	condition := store.AppendCalls[0].Condition
	if condition == nil || condition.After == nil {
		t.Fatal("expected condition with After versionstamp")
	}

	// Should be vs2 from the last event of second read
	if *condition.After != vs2 {
		t.Errorf("expected versionstamp %v (from second read), got %v", vs2, *condition.After)
	}
}

// Test 4: Empty read result
func TestEmptyReadResult_ConditionStillCreated(t *testing.T) {
	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{}, // No events
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify condition is still created (with query but no versionstamp)
	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	condition := store.AppendCalls[0].Condition
	if condition == nil {
		t.Fatal("expected condition even with empty read result")
	}

	// After may be nil since no events were read
	if condition.After != nil {
		t.Logf("After versionstamp is %v (may be nil for empty reads)", condition.After)
	}
}

// Test 5: Handler stops early
func TestHandlerStopsEarly_VersionstampFromLastYielded(t *testing.T) {
	vs1 := dcb.Versionstamp{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	vs2 := dcb.Versionstamp{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	vs3 := dcb.Versionstamp{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"1"}`)}, Position: vs1},
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"2"}`)}, Position: vs2},
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"3"}`)}, Position: vs3},
		},
	}

	impl := func(ctx context.Context, ra fairway.EventReadAppender) error {
		count := 0
		handler := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventA{}),
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			count++
			return count < 2 // Stop after 2 events
		})

		if err := ra.ReadEvents(ctx, handler); err != nil {
			return err
		}

		return ra.AppendEvents(ctx, fairway.Event(TestEventB{Count: count}))
	}

	cmdFunc := commandFunc(impl)
	runner := fairway.NewCommandRunner(store)

	err := runner.RunPure(context.Background(), cmdFunc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Versionstamp should be from 2nd event (vs2), not vs3
	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	condition := store.AppendCalls[0].Condition
	if condition == nil || condition.After == nil {
		t.Fatal("expected condition with After versionstamp")
	}

	if *condition.After != vs2 {
		t.Errorf("expected versionstamp %v (from 2nd event), got %v", vs2, *condition.After)
	}
}

// Helper: command from function
type commandFunc func(context.Context, fairway.EventReadAppender) error

func (f commandFunc) Run(ctx context.Context, ra fairway.EventReadAppender) error {
	return f(ctx, ra)
}

// Test 6: Round-trip serialization
func TestRoundTripSerialization(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	original := TestEventA{Value: "test-value"}
	cmd := &TestCommand{
		T:              t,
		EventsToAppend: []any{original},
		AppendTags:     [][]string{{"tag1", "tag2"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	dcbEvent := store.AppendCalls[0].Events[0]

	// Verify type
	if dcbEvent.Type != "TestEventA" {
		t.Errorf("expected type TestEventA, got %s", dcbEvent.Type)
	}

	// Verify tags
	if len(dcbEvent.Tags) != 2 || dcbEvent.Tags[0] != "tag1" || dcbEvent.Tags[1] != "tag2" {
		t.Errorf("expected tags [tag1, tag2], got %v", dcbEvent.Tags)
	}

	// Verify data contains serialized event
	if len(dcbEvent.Data) == 0 {
		t.Error("expected non-empty data")
	}
}

// Test 7: Custom Typer implementation
func TestCustomTyper(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		EventsToAppend: []any{CustomTypedEvent{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	dcbEvent := store.AppendCalls[0].Events[0]
	if dcbEvent.Type != "custom-type-name" {
		t.Errorf("expected type 'custom-type-name', got %s", dcbEvent.Type)
	}
}

// Test 8: Multiple events preserve order
func TestMultipleEventsPreserveOrder(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T: t,
		EventsToAppend: []any{
			TestEventA{Value: "first"},
			TestEventB{Count: 42},
			TestEventC{Flag: true},
		},
		AppendTags: [][]string{
			{"tag-a"},
			{"tag-b"},
			{"tag-c"},
		},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}

	events := store.AppendCalls[0].Events
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify order and types
	if events[0].Type != "TestEventA" {
		t.Errorf("expected first event type TestEventA, got %s", events[0].Type)
	}
	if events[1].Type != "TestEventB" {
		t.Errorf("expected second event type TestEventB, got %s", events[1].Type)
	}
	if events[2].Type != "TestEventC" {
		t.Errorf("expected third event type TestEventC, got %s", events[2].Type)
	}

	// Verify tags
	if len(events[0].Tags) != 1 || events[0].Tags[0] != "tag-a" {
		t.Errorf("expected first event tags [tag-a], got %v", events[0].Tags)
	}
}

// Test 9: RunPure executes command
func TestRunPure_ExecutesCommand(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	executed := false
	cmd := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		executed = true
		return nil
	})

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !executed {
		t.Error("expected command to be executed")
	}
}

// Test 10: RunPure propagates command errors
func TestRunPure_PropagatesCommandErrors(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	expectedErr := dcb.ErrInvalidQuery
	cmd := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		return expectedErr
	})

	err := runner.RunPure(context.Background(), cmd)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// Test 11: RunPure propagates read errors
func TestRunPure_PropagatesReadErrors(t *testing.T) {
	expectedErr := dcb.ErrAppendConditionFailed
	store := &MockStore{
		ReadError: expectedErr,
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// Test 12: RunPure propagates append errors
func TestRunPure_PropagatesAppendErrors(t *testing.T) {
	expectedErr := dcb.ErrAppendConditionFailed
	store := &MockStore{
		AppendError: expectedErr,
	}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// Test 13: RunWithEffect passes dependencies
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
			return ra.AppendEvents(ctx, fairway.Event(TestEventA{Value: deps.Value}))
		})
	}

	store := &MockStore{}
	deps := Deps{Value: "test-dep"}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	cmd := &EffectCommand{T: t}
	err := runner.RunWithEffect(context.Background(), impl(cmd))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cmd.ReceivedDeps == nil {
		t.Fatal("expected deps to be passed")
	}

	if cmd.ReceivedDeps.Value != "test-dep" {
		t.Errorf("expected deps.Value='test-dep', got %s", cmd.ReceivedDeps.Value)
	}
}

// Test 14: RunPure works on CommandWithEffectRunner
func TestRunPure_WorksOnCommandWithEffectRunner(t *testing.T) {
	type Deps struct {
		Value string
	}

	store := &MockStore{}
	deps := Deps{Value: "unused"}
	runner := fairway.NewCommandWithEffectRunner(store, deps)

	// Pure command should work
	cmd := &TestCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "pure"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.AppendCalls) != 1 {
		t.Fatalf("expected 1 append call, got %d", len(store.AppendCalls))
	}
}

// Test 15: Different dependency types
func TestDifferentDependencyTypes(t *testing.T) {
	type LoggerDeps struct {
		Name string
	}

	type DatabaseDeps struct {
		ConnString string
	}

	// Logger runner
	store1 := &MockStore{}
	loggerRunner := fairway.NewCommandWithEffectRunner(store1, LoggerDeps{Name: "logger"})

	loggerCmd := commandWithEffectFunc[LoggerDeps](func(ctx context.Context, ra fairway.EventReadAppender, deps LoggerDeps) error {
		if deps.Name != "logger" {
			t.Errorf("expected Name='logger', got %s", deps.Name)
		}
		return nil
	})

	err := loggerRunner.RunWithEffect(context.Background(), loggerCmd)
	if err != nil {
		t.Fatalf("logger command failed: %v", err)
	}

	// Database runner
	store2 := &MockStore{}
	dbRunner := fairway.NewCommandWithEffectRunner(store2, DatabaseDeps{ConnString: "db://host"})

	dbCmd := commandWithEffectFunc[DatabaseDeps](func(ctx context.Context, ra fairway.EventReadAppender, deps DatabaseDeps) error {
		if deps.ConnString != "db://host" {
			t.Errorf("expected ConnString='db://host', got %s", deps.ConnString)
		}
		return nil
	})

	err = dbRunner.RunWithEffect(context.Background(), dbCmd)
	if err != nil {
		t.Fatalf("database command failed: %v", err)
	}
}

// Test 16: Cancelled context during read
func TestCancelledContextDuringRead(t *testing.T) {
	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"test"}`)}, Position: dcb.Versionstamp{}},
		},
	}
	runner := fairway.NewCommandRunner(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// Test 17: Cancelled context during append
func TestCancelledContextDuringAppend(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cmd := &TestCommand{
		T:              t,
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(ctx, cmd)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// Helper: CommandWithEffect from function
type commandWithEffectFunc[Deps any] func(context.Context, fairway.EventReadAppender, Deps) error

func (f commandWithEffectFunc[Deps]) Run(ctx context.Context, ra fairway.EventReadAppender, deps Deps) error {
	return f(ctx, ra, deps)
}

// Test 18: Type-only query
func TestTypeOnlyQuery(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}, TestEventB{}},
		EventsToAppend: []any{TestEventC{Flag: true}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.ReadCalls) != 1 {
		t.Fatalf("expected 1 read call, got %d", len(store.ReadCalls))
	}

	query := store.ReadCalls[0].Query
	if len(query.Items) != 1 {
		t.Fatalf("expected 1 query item, got %d", len(query.Items))
	}

	item := query.Items[0]
	if len(item.Types) != 2 {
		t.Errorf("expected 2 types, got %d", len(item.Types))
	}
	if len(item.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(item.Tags))
	}
}

// Test 19: Tag-only query
func TestTagOnlyQuery(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTags:      []string{"tag1", "tag2"},
		EventsToAppend: []any{TestEventA{Value: "test"}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.ReadCalls) != 1 {
		t.Fatalf("expected 1 read call, got %d", len(store.ReadCalls))
	}

	query := store.ReadCalls[0].Query
	if len(query.Items) != 1 {
		t.Fatalf("expected 1 query item, got %d", len(query.Items))
	}

	item := query.Items[0]
	if len(item.Types) != 0 {
		t.Errorf("expected 0 types, got %d", len(item.Types))
	}
	if len(item.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(item.Tags))
	}
}

// Test 20: Types AND tags query
func TestTypesAndTagsQuery(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	cmd := &TestCommand{
		T:              t,
		ShouldRead:     true,
		QueryTypes:     []any{TestEventA{}},
		QueryTags:      []string{"tag1"},
		EventsToAppend: []any{TestEventB{Count: 1}},
	}

	err := runner.RunPure(context.Background(), cmd)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.ReadCalls) != 1 {
		t.Fatalf("expected 1 read call, got %d", len(store.ReadCalls))
	}

	query := store.ReadCalls[0].Query
	if len(query.Items) != 1 {
		t.Fatalf("expected 1 query item, got %d", len(query.Items))
	}

	item := query.Items[0]
	if len(item.Types) != 1 {
		t.Errorf("expected 1 type, got %d", len(item.Types))
	}
	if len(item.Tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(item.Tags))
	}
}

// Test 21: Multiple QueryItems (OR)
func TestMultipleQueryItems_OR(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	// Build command manually to have multiple query items
	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		handler := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventA{}),
			fairway.NewQueryItem().Types(TestEventB{}),
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			return true
		})

		if err := ra.ReadEvents(ctx, handler); err != nil {
			return err
		}

		return ra.AppendEvents(ctx, fairway.Event(TestEventC{Flag: true}))
	})

	err := runner.RunPure(context.Background(), impl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.ReadCalls) != 1 {
		t.Fatalf("expected 1 read call, got %d", len(store.ReadCalls))
	}

	query := store.ReadCalls[0].Query
	if len(query.Items) != 2 {
		t.Errorf("expected 2 query items (OR), got %d", len(query.Items))
	}
}

// Test 22: Empty append ignored
func TestEmptyAppend_Ignored(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		return ra.AppendEvents(ctx) // Empty append
	})

	err := runner.RunPure(context.Background(), impl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// No append call should be recorded
	if len(store.AppendCalls) != 0 {
		t.Errorf("expected 0 append calls, got %d", len(store.AppendCalls))
	}
}

// Test 23: ReadEvents with nil handler
func TestReadEvents_NilHandler(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		// Handler with nil Handle function
		handler := &fairway.EventHandler{}
		return ra.ReadEvents(ctx, handler)
	})

	err := runner.RunPure(context.Background(), impl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// Test 24: Multiple appends in one command
func TestMultipleAppends_InOneCommand(t *testing.T) {
	store := &MockStore{}
	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		if err := ra.AppendEvents(ctx, fairway.Event(TestEventA{Value: "first"})); err != nil {
			return err
		}
		return ra.AppendEvents(ctx, fairway.Event(TestEventB{Count: 2}))
	})

	err := runner.RunPure(context.Background(), impl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Both appends should be captured
	if len(store.AppendCalls) != 2 {
		t.Fatalf("expected 2 append calls, got %d", len(store.AppendCalls))
	}

	// Both should have same condition (nil in this case, no read)
	if store.AppendCalls[0].Condition != nil {
		t.Error("expected first append to have nil condition")
	}
	if store.AppendCalls[1].Condition != nil {
		t.Error("expected second append to have nil condition")
	}
}

// Test 25: Read-Append-Read-Append
func TestReadAppendReadAppend(t *testing.T) {
	vs1 := dcb.Versionstamp{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	vs2 := dcb.Versionstamp{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

	store := &MockStore{
		ReadEvents: []dcb.StoredEvent{
			{Event: dcb.Event{Type: "TestEventA", Data: []byte(`{"Value":"a"}`)}, Position: vs1},
			{Event: dcb.Event{Type: "TestEventB", Data: []byte(`{"Count":1}`)}, Position: vs2},
		},
	}

	runner := fairway.NewCommandRunner(store)

	impl := commandFunc(func(ctx context.Context, ra fairway.EventReadAppender) error {
		// First read - register both types to handle all events from mock
		handler1 := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventA{}, TestEventB{}),
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			return true
		})
		if err := ra.ReadEvents(ctx, handler1); err != nil {
			return err
		}

		// First append
		if err := ra.AppendEvents(ctx, fairway.Event(TestEventB{Count: 100})); err != nil {
			return err
		}

		// Second read (should update versionstamp)
		handler2 := fairway.QueryItems(
			fairway.NewQueryItem().Types(TestEventB{}),
		).Handle(func(te fairway.TaggedEvent, err error) bool {
			return true
		})
		if err := ra.ReadEvents(ctx, handler2); err != nil {
			return err
		}

		// Second append
		return ra.AppendEvents(ctx, fairway.Event(TestEventC{Flag: true}))
	})

	err := runner.RunPure(context.Background(), impl)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(store.AppendCalls) != 2 {
		t.Fatalf("expected 2 append calls, got %d", len(store.AppendCalls))
	}

	// First append should use vs2 from first read (last event seen)
	cond1 := store.AppendCalls[0].Condition
	if cond1 == nil || cond1.After == nil {
		t.Fatal("expected first append to have condition with versionstamp")
	}
	if *cond1.After != vs2 {
		t.Errorf("expected first append After=%v, got %v", vs2, *cond1.After)
	}

	// Second append should also use vs2 from second read (both events returned by mock)
	cond2 := store.AppendCalls[1].Condition
	if cond2 == nil || cond2.After == nil {
		t.Fatal("expected second append to have condition with versionstamp")
	}
	if *cond2.After != vs2 {
		t.Errorf("expected second append After=%v, got %v", vs2, *cond2.After)
	}
}
