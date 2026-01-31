package fairway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/err0r500/fairway/dcb"
)

// PURE COMMANDS
// Command is a pure command without side effects
type Command interface {
	Run(ctx context.Context, ra EventReadAppender) error
}

// RetryableCommand is an optional interface that commands can implement
// to provide custom retry configuration
type RetryableCommand interface {
	Command
	RetryOptions() []retry.Option
}

// CommandRunner runs pure Commands
type CommandRunner interface {
	RunPure(ctx context.Context, command Command) error
}

// commandRunner is the concrete implementation of CommandRunner
type commandRunner struct {
	store     dcb.DcbStore
	retryOpts []retry.Option
}

// CommandRunnerOption configures CommandRunner
type CommandRunnerOption func(*commandRunner)

// WithRetryOptions configures retry behavior using retry-go options
func WithRetryOptions(opts ...retry.Option) CommandRunnerOption {
	return func(cr *commandRunner) {
		cr.retryOpts = opts
	}
}

// NewCommandRunner creates a command runner
// By default, retries 3 times with exponential backoff on ErrAppendConditionFailed.
// Pass WithRetryOptions() to customize or disable (use retry.Attempts(1) for no retry).
func NewCommandRunner(store dcb.DcbStore, opts ...CommandRunnerOption) CommandRunner {
	cr := &commandRunner{
		store: store,
		retryOpts: []retry.Option{
			retry.Attempts(4), // initial attempt + 3 retries
			retry.Delay(10 * time.Millisecond),
			retry.DelayType(retry.BackOffDelay),
			retry.MaxDelay(500 * time.Millisecond),
			retry.RetryIf(func(err error) bool {
				// Only retry on append condition failed
				return errors.Is(err, dcb.ErrAppendConditionFailed)
			}),
		},
	}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

// RunPure executes a command with automatic retry on ErrAppendConditionFailed
// Priority: command-level config > runner-level config
func (cr *commandRunner) RunPure(ctx context.Context, cmd Command) error {
	// Check if command provides custom retry options
	opts := cr.retryOpts
	if retryable, ok := cmd.(RetryableCommand); ok {
		opts = retryable.RetryOptions()
	}

	return retry.Do(func() error {
		return cmd.Run(ctx, newReadAppender(cr.store))
	}, opts...)
}

// COMMANDS WITH SIDE EFFECTS
// CommandWithEffect represents a command that can perform side effects
// using injected dependencies, while also interacting with the event store
type CommandWithEffect[Deps any] interface {
	Run(ctx context.Context, ra EventReadAppenderExtended, deps Deps) error
}

// CommandWithEffectRunner runs commands with side effects and dependency injection.
// It can run both pure commands (via RunPure) and commands with side effects (via RunWithEffect).
type CommandWithEffectRunner[Deps any] interface {
	CommandRunner
	RunWithEffect(ctx context.Context, command CommandWithEffect[Deps]) error
}

// commandWithEffectRunner is the concrete implementation of CommandWithEffectRunner
type commandWithEffectRunner[Deps any] struct {
	store     dcb.DcbStore
	deps      Deps
	retryOpts []retry.Option
}

// CommandWithEffectRunnerOption configures CommandWithEffectRunner
type CommandWithEffectRunnerOption[Deps any] func(*commandWithEffectRunner[Deps])

// WithRetryOptionsForEffect configures retry behavior using retry-go options
func WithRetryOptionsForEffect[Deps any](opts ...retry.Option) CommandWithEffectRunnerOption[Deps] {
	return func(cr *commandWithEffectRunner[Deps]) {
		cr.retryOpts = opts
	}
}

// NewCommandWithEffectRunner creates a command runner with dependency injection
// By default, NO RETRY (side effects may not be idempotent).
// Use WithRetryOptionsForEffect() to enable retry when safe.
func NewCommandWithEffectRunner[Deps any](store dcb.DcbStore, deps Deps, opts ...CommandWithEffectRunnerOption[Deps]) CommandWithEffectRunner[Deps] {
	cr := &commandWithEffectRunner[Deps]{
		store: store,
		deps:  deps,
		retryOpts: []retry.Option{
			retry.Attempts(1), // No retry by default
		},
	}
	for _, opt := range opts {
		opt(cr)
	}
	return cr
}

// RunPure executes a pure command (deps are not needed)
// Priority: command-level config > runner-level config
func (cr *commandWithEffectRunner[Deps]) RunPure(ctx context.Context, cmd Command) error {
	// Check if command provides custom retry options
	opts := cr.retryOpts
	if retryable, ok := cmd.(RetryableCommand); ok {
		opts = retryable.RetryOptions()
	}

	return retry.Do(func() error {
		return cmd.Run(ctx, newReadAppender(cr.store))
	}, opts...)
}

// RunWithEffect executes a command with side effects using injected dependencies
// Priority: command-level config > runner-level config
func (cr *commandWithEffectRunner[Deps]) RunWithEffect(ctx context.Context, cmd CommandWithEffect[Deps]) error {
	// Check if command provides custom retry options
	opts := cr.retryOpts
	if retryable, ok := cmd.(interface {
		RetryOptions() []retry.Option
	}); ok {
		opts = retryable.RetryOptions()
	}

	return retry.Do(func() error {
		return cmd.Run(ctx, newReadAppenderExtended(cr.store), cr.deps)
	}, opts...)
}

type EventReadAppender interface {
	EventsReader
	AppendEvents(ctx context.Context, event Event, remainingEvents ...Event) error
}

type EventReadAppenderExtended interface {
	EventReadAppender
	AppendEventsNoCondition(ctx context.Context, event Event, remainingEvents ...Event) error
}

// commandReadAppender provides read-then-conditional-append for commands
type commandReadAppender struct {
	lastSeenVersionstamp *dcb.Versionstamp
	store                dcb.DcbStore
	query                *dcb.Query
	eventRegistry        eventRegistry
}

// newReadAppender creates a ReadAppender with given store
// it tracks the last versionstamp consumed by the command
// and injects it directly when using append
func newReadAppender(store dcb.DcbStore) EventReadAppender {
	return newReadAppenderExtended(store)
}

// newReadAppender creates a ReadAppender with given store
// it tracks the last versionstamp consumed by the command
// and injects it directly when using append
func newReadAppenderExtended(store dcb.DcbStore) EventReadAppenderExtended {
	return &commandReadAppender{
		store:         store,
		eventRegistry: newEventRegistry(),
	}
}

// ReadEvents reads events using the eventHandler's query and dispatches to handlers
func (ra *commandReadAppender) ReadEvents(ctx context.Context, query Query, handler HandlerFunc) error {
	if handler == nil {
		return nil
	}

	// Auto-register types from query
	for _, item := range query.items {
		ra.eventRegistry.registerTypes(item.typeRegistry)
	}

	// Convert fairway Query to dcb Query
	ra.query = query.toDcb()

	for dcbStoredEvent, err := range ra.store.Read(ctx, *ra.query, nil) {
		if err != nil {
			// context errors already have context
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("reading events: %s", err)
		}

		// Track last versionstamp
		ra.lastSeenVersionstamp = &dcbStoredEvent.Position

		// Deserialize dcb.Event → Event
		ev, err := ra.eventRegistry.deserialize(dcbStoredEvent.Event)
		if err != nil {
			return fmt.Errorf("deserializing event at position %x: %s", dcbStoredEvent.Position[:], err)
		}

		// Dispatch Event to handler
		if !handler(ev) {
			return nil
		}
	}

	return nil
}

// AppendEventsNoCondition appends events without any condition (even if there was a Read previously)
func (ra *commandReadAppender) AppendEventsNoCondition(ctx context.Context, event Event, remainingEvents ...Event) error {
	dcbEvents, err := serializeEvents(append([]Event{event}, remainingEvents...))
	if err != nil {
		return err
	}

	return ra.store.Append(ctx, dcbEvents, nil)
}

// AppendEvents appends events with conditional check using tracked versionstamp
func (ra *commandReadAppender) AppendEvents(ctx context.Context, event Event, remainingEvents ...Event) error {
	// Serialize Event → dcb.Event
	dcbEvents, err := serializeEvents(append([]Event{event}, remainingEvents...))
	if err != nil {
		return err
	}

	// Build condition using query if used
	// (some commands may just append Event(s) without reading anything)
	if ra.query == nil {
		return ra.store.Append(ctx, dcbEvents, nil)
	}

	return ra.store.Append(ctx, dcbEvents,
		&dcb.AppendCondition{
			Query: *ra.query,
			After: ra.lastSeenVersionstamp,
		})
}

func serializeEvents(events []Event) ([]dcb.Event, error) {
	dcbEvents := make([]dcb.Event, len(events))
	for i, ev := range events {
		dcbEvent, err := ToDcbEvent(ev)
		if err != nil {
			return nil, err
		}
		dcbEvents[i] = dcbEvent
	}

	return dcbEvents, nil
}
