<p align="center">
  <img src="./doc/fairway.png">
</p>

<p align="center">
  <strong>A Go framework for building micromodule backends with event sourcing.</strong><br>
  Tiny, independent modules. One shared event log. Zero coupling.
</p>

<p align="center">
  <a href="#concepts">Concepts</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#patterns">Patterns</a> ·
  <a href="#full-example">Full Example</a> ·
  <a href="#internal-packages">Internals</a>
</p>

---

> ⚠️ **Experimental** — under heavy development, not yet published. Clone and try it locally.

---

## What is Fairway?

Fairway is a Go framework for building backends from small, self-contained modules that communicate exclusively through a shared event log. Each module does one thing and owns only the state it needs to do it.

It is built on two foundations:

- **[Dynamic Consistency Boundaries (DCB)](https://dcb.events)** — a model for event sourcing where consistency is scoped to the data a command actually reads, not to an entire aggregate.
- **[FoundationDB](https://www.foundationdb.org)** — a distributed, ACID-compliant key-value store that handles events, queues, and read models in a single datastore.

---

## Why Fairway?

| Problem with traditional approaches | How Fairway addresses it |
|---|---|
| Shared domain models create coupling | Each command defines only the minimal model it needs |
| Aggregates cause unnecessary contention | Optimistic locking covers only what a command actually reads |
| Refactoring streams requires migrations | Events are stored flat; views reinterpret them, no migration needed |
| Multiple databases to operate | FoundationDB handles events, queues, and read models |
| Merge conflicts on shared code | Commands share no code; self-register via `init()` |

---

## Concepts

### Micromodules

The UNIX philosophy applied to backends: system behavior emerges from tiny, independent modules composed through a shared event log.

Each module:
- **Does one thing** — changes one small part of the system, or displays one slice of information
- **Is disposable** — replaceable at any time without breakage or migration
- **Owns minimal state** — only what its specific task requires
- **Never talks to other modules** — all composition happens through the shared log

### Three Patterns

Every module implements exactly one of these patterns (from [Event Modeling](https://eventmodeling.org/)):

```
User action          Event log        Projection
    │                    │                │
    ▼                    ▼                ▼
┌──────────┐    ┌──────────────┐    ┌──────────┐
│ Command  │───▶│    Events    │───▶│   View   │
└──────────┘    └──────┬───────┘    └──────────┘
                       │
               ┌───────▼──────┐
               │  Automation  │  (View → Command, no user)
               └──────────────┘
```

---

## Quick Start

**Prerequisites:** Go 1.24+, FoundationDB installed and running.

```bash
# Clone and try the example
git clone https://github.com/err0r500/fairway
cd fairway/examples/todolist
go generate ./...
go run .
```

Then:
```bash
curl -X POST http://localhost:8080/api/lists/my-list \
     -H "Content-Type: application/json" \
     -d '{"name": "Shopping"}'

curl http://localhost:8080/api/lists/my-list
```

---

## Patterns

### 1. Event

An event is a plain Go struct. Implement `Tags()` to scope it to a specific entity.

```go
type ListCreated struct {
    ListId string `json:"listId"`
    Name   string `json:"name"`
}

func (e ListCreated) Tags() []string {
    return []string{"list:" + e.ListId}
}
```

---

### 2. Command

A command reads what it needs, decides, then appends. The framework retries on optimistic locking conflicts — scoped only to what was actually read.

```go
type createListCommand struct {
    listId string
    name   string
}

func (cmd createListCommand) Run(ctx context.Context, ev fairway.EventReadAppender) error {
    // Read: check if the list already exists
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

    // Append: emit the event
    return ev.AppendEvents(ctx, fairway.NewEvent(ListCreated{
        ListId: cmd.listId,
        Name:   cmd.name,
    }))
}
```

Wire it to HTTP with a single registration:

```go
func Register(registry *fairway.HttpChangeRegistry) {
    registry.RegisterCommand("POST /api/lists/{listId}", func(runner fairway.CommandRunner) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            if err := runner.RunPure(r.Context(), createListCommand{
                listId: r.PathValue("listId"),
                name:   /* parse body */,
            }); err != nil {
                w.WriteHeader(http.StatusInternalServerError)
                return
            }
            w.WriteHeader(http.StatusCreated)
        }
    })
}
```

Self-register from `init()` so the module wires itself with zero coordination:

```go
func init() { Register(&change.ChangeRegistry) }
```

---

### 3. View

A view reads events and builds a projection on the fly. No stored state, no cache invalidation.

```go
func Register(registry *fairway.HttpViewRegistry) {
    registry.RegisterView("GET /api/lists/{listId}", func(reader fairway.EventsReader) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            list := struct {
                Id         string `json:"id"`
                Name       string `json:"name"`
                ItemsCount uint   `json:"itemsCount"`
            }{}

            reader.ReadEvents(r.Context(),
                fairway.QueryItems(
                    fairway.NewQueryItem().
                        Types(ListCreated{}, ItemCreated{}).
                        Tags("list:" + r.PathValue("listId")),
                ),
                func(e fairway.Event) bool {
                    switch data := e.Data.(type) {
                    case ListCreated:
                        list.Id, list.Name = data.ListId, data.Name
                    case ItemCreated:
                        list.ItemsCount++
                    }
                    return true
                })

            json.NewEncoder(w).Encode(list)
        }
    })
}
```

---

### 4. Automation

An automation watches the event log and triggers commands without user intervention — useful for side effects, notifications, or derived workflows.

```go
automation, _ := fairway.NewAutomation(
    store,
    emailDeps,
    "send-welcome-email",   // queue name (unique per automation)
    UserRegistered{},       // event type to watch
    func(ev fairway.Event) fairway.CommandWithEffect[EmailDeps] {
        return &sendWelcomeEmailCommand{Event: ev}
    },
    fairway.WithNumWorkers(4),
    fairway.WithMaxAttempts(3),
)

automation.Start(ctx)
defer automation.Stop()
```

---

## Full Example

The [`examples/todolist`](./examples/todolist) directory contains a complete working application:

```
examples/todolist/
├── event/          # Event types (ListCreated, ItemCreated)
├── change/
│   ├── createlist/ # POST /api/lists/{listId}
│   └── additem/    # POST /api/lists/{listId}/items/{itemId}
├── view/
│   └── showlist/   # GET  /api/lists/{listId}
└── main.go         # Wire up FDB, router, and start server
```

The `main.go` is minimal — each module registers itself:

```go
func main() {
    fdb.MustAPIVersion(740)
    db := fdb.MustOpenDefault()
    store := dcb.NewDcbStore(db, "core")
    mux := http.NewServeMux()

    change.ChangeRegistry.RegisterRoutes(mux, fairway.NewCommandRunner(store))
    view.ViewRegistry.RegisterRoutes(mux, fairway.NewReader(store))

    log.Fatal(http.ListenAndServe(":8080", mux))
}
```

---

## Internal Packages

| Package | Description |
|---|---|
| [`dcb/`](./dcb/) | Low-level DCB-compliant event store backed by FoundationDB |
| `fairway` (root) | Command runner, event reader, automation, HTTP registries |
| `utils/` | HTTP helpers: JSON parsing, idempotency middleware |
| `testing/` | Test utilities: `given`, `when`, `then` helpers |

---

## Development

To run tests, FoundationDB must be available. Set the build tag:

```bash
export GOFLAGS="-tags=test"
go test ./...
```

---

## Status

- ✅ `dcb/` — DCB-compliant event store backed by FoundationDB
- 🔄 High-level modules — commands, views, automations, HTTP wiring
- 📋 CLI — code generation tools (planned)

---

> Is it a good idea?  I'm not sure yet.
> Is it worth exploring?  Absolutely, yes.
