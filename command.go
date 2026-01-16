package fairway

import (
	"context"

	"github.com/err0r500/fairway/dcb"
)

// Command is a pure command without side effects
type Command interface {
	Run(ctx context.Context, ra *ReadAppender) error
}

// CommandRunner runs pure Commands
type CommandRunner interface {
	Run(ctx context.Context, command Command) error
}

// CommandWithEffect represents a command that can perform side effects
// using injected dependencies, while also interacting with the event store
type CommandWithEffect[Deps any] interface {
	Run(ctx context.Context, ra *ReadAppender, deps Deps) error
}

// CommandWithEffectRunner runs commands with side effects and dependency injection.
// It can run both pure commands (via RunPure) and commands with side effects (via RunWithEffect).
type CommandWithEffectRunner[Deps any] interface {
	RunPure(ctx context.Context, command Command) error
	RunWithEffect(ctx context.Context, command CommandWithEffect[Deps]) error
}

// ReadAppender provides read-then-conditional-append for commands
type ReadAppender struct {
	lastSeenVersionstamp *dcb.Versionstamp
	store                dcb.DcbStore
	query                *dcb.Query
	eventRegistry        eventRegistry
}

// NewReadAppender creates a ReadAppender with given store
func NewReadAppender(store dcb.DcbStore) *ReadAppender {
	return &ReadAppender{
		store:         store,
		eventRegistry: newEventRegistry(),
	}
}

// ReadEvents reads events using the router's query and dispatches to handlers
func (ra *ReadAppender) ReadEvents(ctx context.Context, router *EventHandler) error {
	if router.handle == nil {
		return nil
	}

	// Auto-register types from query
	for _, item := range router.query.Items {
		for _, instance := range item.eventInstances {
			ra.eventRegistry.register(instance)
		}
	}

	// Convert fairway Query to dcb Query
	ra.query = router.query.toDcb()

	for dcbStoredEvent, err := range ra.store.Read(ctx, *ra.query, nil) {
		if err != nil {
			return err
		}

		// Check context
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Track last versionstamp
		ra.lastSeenVersionstamp = &dcbStoredEvent.Position

		// Deserialize dcb.Event → event
		fairwayEvent, err := ra.eventRegistry.deserialize(dcbStoredEvent.Event)
		if err != nil {
			return err
		}

		// Dispatch TaggedEvent to handler
		if !router.handle(TaggedEvent{Event: fairwayEvent, Tags: dcbStoredEvent.Tags}, nil) {
			return nil
		}
	}

	return nil
}

// AppendEvents appends events with conditional check using tracked versionstamp
func (ra *ReadAppender) AppendEvents(ctx context.Context, events ...TaggedEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Serialize TaggedEvent → dcb.Event
	dcbEvents := make([]dcb.Event, len(events))
	for i, taggedEvt := range events {
		dcbEvent, err := toDcbEvent(taggedEvt)
		if err != nil {
			return err
		}
		dcbEvents[i] = dcbEvent
	}

	// Build condition using query and last versionstamp
	if ra.query == nil {
		return ra.store.Append(ctx, dcbEvents, nil)
	}

	return ra.store.Append(ctx, dcbEvents,
		&dcb.AppendCondition{
			Query: *ra.query,
			After: ra.lastSeenVersionstamp,
		})
}

// commandRunner is the concrete implementation of CommandRunner
type commandRunner struct {
	store dcb.DcbStore
}

// NewCommandRunner creates a command runner
func NewCommandRunner(store dcb.DcbStore) CommandRunner {
	return &commandRunner{
		store: store,
	}
}

// Run executes a command
func (cr *commandRunner) Run(ctx context.Context, cmd Command) error {
	return cmd.Run(ctx, NewReadAppender(cr.store))
}

// commandWithEffectRunner is the concrete implementation of CommandWithEffectRunner
type commandWithEffectRunner[Deps any] struct {
	store dcb.DcbStore
	deps  Deps
}

// NewCommandWithEffectRunner creates a command runner with dependency injection
func NewCommandWithEffectRunner[Deps any](store dcb.DcbStore, deps Deps) CommandWithEffectRunner[Deps] {
	return &commandWithEffectRunner[Deps]{
		store: store,
		deps:  deps,
	}
}

// RunPure executes a pure command (deps are not needed)
func (cr *commandWithEffectRunner[Deps]) RunPure(ctx context.Context, cmd Command) error {
	return cmd.Run(ctx, NewReadAppender(cr.store))
}

// RunWithEffect executes a command with side effects using injected dependencies
func (cr *commandWithEffectRunner[Deps]) RunWithEffect(ctx context.Context, cmd CommandWithEffect[Deps]) error {
	return cmd.Run(ctx, NewReadAppender(cr.store), cr.deps)
}
