package fairway

import (
	"context"

	"github.com/err0r500/fairway/dcb"
)

// PURE COMMANDS
// Command is a pure command without side effects
type Command interface {
	Run(ctx context.Context, ra EventReadAppender) error
}

// CommandRunner runs pure Commands
type CommandRunner interface {
	RunPure(ctx context.Context, command Command) error
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

// RunPure executes a command
func (cr *commandRunner) RunPure(ctx context.Context, cmd Command) error {
	return cmd.Run(ctx, newReadAppender(cr.store))
}

// COMMANDS WITH SIDE EFFECTS
// CommandWithEffect represents a command that can perform side effects
// using injected dependencies, while also interacting with the event store
type CommandWithEffect[Deps any] interface {
	Run(ctx context.Context, ra EventReadAppender, deps Deps) error
}

// CommandWithEffectRunner runs commands with side effects and dependency injection.
// It can run both pure commands (via RunPure) and commands with side effects (via RunWithEffect).
type CommandWithEffectRunner[Deps any] interface {
	CommandRunner
	RunWithEffect(ctx context.Context, command CommandWithEffect[Deps]) error
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
	return cmd.Run(ctx, newReadAppender(cr.store))
}

// RunWithEffect executes a command with side effects using injected dependencies
func (cr *commandWithEffectRunner[Deps]) RunWithEffect(ctx context.Context, cmd CommandWithEffect[Deps]) error {
	return cmd.Run(ctx, newReadAppender(cr.store), cr.deps)
}

type EventReadAppender interface {
	EventsReader
	AppendEvents(ctx context.Context, events ...TaggedEvent) error
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
		if !handler(TaggedEvent{Event: fairwayEvent, Tags: dcbStoredEvent.Tags}, nil) {
			return nil
		}
	}

	return nil
}

// AppendEvents appends events with conditional check using tracked versionstamp
func (ra *commandReadAppender) AppendEvents(ctx context.Context, events ...TaggedEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Serialize TaggedEvent → dcb.Event
	dcbEvents := make([]dcb.Event, len(events))
	for i, taggedEvt := range events {
		dcbEvent, err := ToDcbEvent(taggedEvt)
		if err != nil {
			return err
		}
		dcbEvents[i] = dcbEvent
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
