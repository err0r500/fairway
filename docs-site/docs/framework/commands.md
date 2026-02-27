# Commands

Commands are the write side of Fairway. A command reads the events relevant to its decision, then conditionally appends new events. If a concurrent writer appended a matching event in the meantime, the command is retried automatically.

---

## The `Command` Interface

```go
type Command interface {
    Run(ctx context.Context, ra EventReadAppender) error
}
```

`EventReadAppender` gives the command two operations:

```go
type EventReadAppender interface {
    EventsReader  // ReadEvents(ctx, query, handler) error
    AppendEvents(ctx context.Context, event Event, rest ...Event) error
}
```

### Lifecycle Inside `Run`

1. Call `ReadEvents(ctx, query, handler)` — reads events
2. Make a decision based on what was read
3. Call `AppendEvents(ctx, newEvent)` — appends new events

**Under the hood:** every `ReadEvents` call is tracked. When `AppendEvents` runs, it builds an `AppendCondition` from all tracked reads. The append succeeds only if no matching events were written since the reads — guaranteeing the decision is still valid.

If a concurrent write invalidates the decision, `AppendEvents` returns `ErrAppendConditionFailed` and the runner retries from scratch.

---

## Example Command

```go
type createListCommand struct {
    listId string
    name   string
}

func (cmd createListCommand) Run(ctx context.Context, ev fairway.EventReadAppender) error {
    alreadyExists := false

    ev.ReadEvents(ctx,
        fairway.QueryItems(
            fairway.NewQueryItem().
                Types(ListCreated{}).
                Tags("list:" + cmd.listId),
        ),
        func(e fairway.Event) bool {
            alreadyExists = true
            return false // stop on first match
        })

    if alreadyExists {
        return errors.New("list already exists")
    }

    return ev.AppendEvents(ctx, fairway.NewEvent(ListCreated{
        ListId: cmd.listId,
        Name:   cmd.name,
    }))
}
```

---

## `CommandRunner`

```go
type CommandRunner interface {
    RunPure(ctx context.Context, command Command) error
}
```

### Creating a Runner

```go
runner := fairway.NewCommandRunner(store)
```

By default, the runner retries up to 3 times (4 total attempts) with exponential backoff (10ms base, max 500ms) on `ErrAppendConditionFailed`.

### Custom Retry Options

```go
runner := fairway.NewCommandRunner(store,
    fairway.WithRetryOptions(
        retry.Attempts(5),
        retry.Delay(50 * time.Millisecond),
        retry.DelayType(retry.BackOffDelay),
    ),
)
```

Use `retry.Attempts(1)` to disable retries entirely.

### Per-Command Retry

Implement `RetryableCommand` to override retry behaviour per command:

```go
type RetryableCommand interface {
    Command
    RetryOptions() []retry.Option
}
```

```go
func (cmd createListCommand) RetryOptions() []retry.Option {
    return []retry.Option{retry.Attempts(1)} // no retry for this command
}
```

---

## Append Without Prior Read

A command can append events without reading anything first. In this case no conditional check is applied:

```go
func (cmd logEvent) Run(ctx context.Context, ev fairway.EventReadAppender) error {
    // No ReadEvents call — AppendEvents has no condition
    return ev.AppendEvents(ctx, fairway.NewEvent(AuditLogEntry{Action: "login"}))
}
```

---

## Retry Flow Diagram

```
RunPure(cmd)
    │
    └─► cmd.Run(ctx, readAppender)
            │
            ├── ReadEvents(query1)       ← tracked
            ├── ReadEvents(query2)       ← tracked
            │
            ├── [decision logic]
            │
            └── AppendEvents(event)
                    │
                    ├── builds AppendCondition from query1 + query2
                    │
                    ├── OK → return nil
                    │
                    └── ErrAppendConditionFailed (decision invalidated)
                            │
                            └─► retry (up to 3 times)
                                    └─► cmd.Run(ctx, fresh readAppender)
```

