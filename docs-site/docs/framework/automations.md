# Automations

Automations watch the event log and trigger commands without user interaction. They are useful for side effects, notifications, derived workflows, and any logic that should fire in response to an event rather than to an HTTP request.

---

## Concept

An automation is a background worker that:

1. Watches for events of a specific type
2. For each new event, creates a `CommandWithEffect` and runs it
3. Handles retries and, after exhausting them, moves the event to a dead-letter queue (DLQ)

All coordination state (cursor position, job leases, DLQ entries) is stored in FoundationDB alongside your events — no extra infrastructure needed.

---

## `Startable` Interface

```go
type Startable interface {
    QueueId() string
    Start(ctx context.Context) error
    Stop()
    Wait() error
}
```

Every `Automation` implements `Startable`. `Start` launches background goroutines; `Stop` signals them to exit; `Wait` blocks until they do.

---

## `Automation[Deps]`

```go
type Automation[Deps any] struct { /* ... */ }
```

Generic over `Deps` — the dependency struct injected into each command.

### Creating an Automation

```go
automation, err := fairway.NewAutomation(
    store,
    EmailDeps{Mailer: mailer},   // deps injected into every command
    "send-welcome-email",        // unique queue ID
    UserRegistered{},            // event type to watch (zero value)
    func(ev fairway.Event) fairway.CommandWithEffect[EmailDeps] {
        return &sendWelcomeEmailCommand{Event: ev}
    },
    fairway.WithNumWorkers[EmailDeps](4),
    fairway.WithMaxAttempts[EmailDeps](3),
)
```

### Starting and Stopping

```go
if err := automation.Start(ctx); err != nil {
    log.Fatal(err)
}
defer func() {
    automation.Stop()
    automation.Wait()
}()
```

---

## Configuration Options

| Option | Default | Description |
|---|---|---|
| `WithNumWorkers(n)` | 1 | Number of parallel worker goroutines |
| `WithLeaseTTL(d)` | 30s | How long a worker holds a job lease |
| `WithGracePeriod(d)` | 60s | Grace period before a stale lease is reclaimed |
| `WithMaxAttempts(n)` | 3 | Max attempts before a job goes to DLQ |
| `WithBatchSize(n)` | 16 | Events fetched per poll cycle |
| `WithPollInterval(d)` | 100ms | How often to check for new events |
| `WithRetryBaseWait(d)` | 1min | Base backoff wait between retries |

All options are typed generics — pass the `Deps` type parameter explicitly:

```go
fairway.WithNumWorkers[EmailDeps](4)
```

---

## `AutomationRegistry`

Use `AutomationRegistry` to manage multiple automations as a group:

```go
type AutomationRegistry[Deps any] struct { /* ... */ }

func (r *AutomationRegistry[Deps]) RegisterAutomation(f AutomationFactory[Deps])
func (r *AutomationRegistry[Deps]) StartAll(ctx context.Context, store dcb.DcbStore, deps Deps) (stopFn func(), error)
```

### Example

```go
var AutomationReg fairway.AutomationRegistry[AppDeps]

func init() {
    AutomationReg.RegisterAutomation(func(store dcb.DcbStore, deps AppDeps) (fairway.Startable, error) {
        return fairway.NewAutomation(store, deps, "send-welcome-email", UserRegistered{},
            func(ev fairway.Event) fairway.CommandWithEffect[AppDeps] {
                return &sendWelcomeEmailCommand{Event: ev}
            },
        )
    })
}
```

In `main.go`:

```go
stop, err := AutomationReg.StartAll(ctx, store, deps)
if err != nil {
    log.Fatal(err)
}
defer stop()
```

---

## How It Works Internally

### Cursor

Each automation maintains a cursor in FDB (`namespace/queueId/cursor`). The cursor points to the last versionstamp processed. On each poll, the automation reads new events after the cursor, enqueues them as jobs.

### Job Queue

Jobs are stored as FDB keys in `namespace/queueId/queue/`. Workers claim jobs by writing a lease (with TTL). If a worker crashes, the lease expires and another worker picks up the job.

### Dead-Letter Queue (DLQ)

After `MaxAttempts` failures, a job is moved to `namespace/queueId/dlq/`. Jobs in the DLQ are not retried automatically. Monitor and inspect via FDB tooling.

### Error Monitoring

```go
// Non-blocking error channel
for err := range automation.Errors() {
    log.Printf("automation error: %v", err)
}
```
