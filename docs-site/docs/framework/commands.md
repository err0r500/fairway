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

1. Call `ReadEvents(ctx, query, handler)` — reads events and tracks the last versionstamp.
2. Make a decision based on what was read.
3. Call `AppendEvents(ctx, newEvent)` — appends with a conditional check using the tracked versionstamp.

If another writer appended a matching event between steps 1 and 3, `AppendEvents` returns `ErrAppendConditionFailed`. The runner retries the entire `Run` from scratch.

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

## Commands with Side Effects

When a command needs to interact with external systems (sending email, calling an API), use `CommandWithEffect`:

```go
type CommandWithEffect[Deps any] interface {
    Run(ctx context.Context, ra EventReadAppenderExtended, deps Deps) error
}
```

`Deps` is injected at runner creation time. `EventReadAppenderExtended` extends `EventReadAppender` with:

```go
type EventReadAppenderExtended interface {
    EventReadAppender
    AppendEventsNoCondition(ctx context.Context, event Event, rest ...Event) error
}
```

`AppendEventsNoCondition` appends without a conditional guard — useful when a side-effecting command must not retry its append (since the side effect already happened).

### `CommandWithEffectRunner`

```go
type CommandWithEffectRunner[Deps any] interface {
    CommandRunner                          // RunPure is still available
    RunWithEffect(ctx context.Context, command CommandWithEffect[Deps]) error
}
```

```go
type EmailDeps struct {
    Mailer *smtp.Client
}

runner := fairway.NewCommandWithEffectRunner(store, EmailDeps{Mailer: mailer})

err := runner.RunWithEffect(ctx, &sendWelcomeEmailCommand{userId: "42"})
```

!!! warning "No retry by default for side effects"
    `CommandWithEffectRunner` defaults to no retry (`Attempts(1)`) because side effects (sending email, charging a card) may not be safe to repeat. Enable retry explicitly only when your side effects are idempotent.

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
            ├── ReadEvents(query)        ← tracks lastVersionstamp
            │
            ├── [decision logic]
            │
            └── AppendEvents(event)
                    │
                    ├── OK → return nil
                    │
                    └── ErrAppendConditionFailed
                            │
                            └─► retry (up to 3 times)
                                    └─► cmd.Run(ctx, fresh readAppender)
```
