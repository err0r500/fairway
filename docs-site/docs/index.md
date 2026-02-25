# Fairway

**A Go framework for building micromodule backends with event sourcing.**
Tiny, independent modules. One shared event log. Zero coupling.

---

!!! warning "Experimental"
    Fairway is under heavy development and not yet published. Clone and try it locally.

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

## Architecture

The abstraction stack, from lowest to highest:

```
┌─────────────────────────────────────────┐
│         Testing Helpers                 │  given / when / then
├─────────────────────────────────────────┤
│         HTTP Layer                      │  HttpChangeRegistry / HttpViewRegistry
├─────────────────────────────────────────┤
│         Framework Layer                 │  Command / View / Automation
├─────────────────────────────────────────┤
│         Event & Query Layer             │  Event / Query / QueryItem
├─────────────────────────────────────────┤
│         DCB Store                       │  DcbStore / Append / Read
├─────────────────────────────────────────┤
│         FoundationDB                    │
└─────────────────────────────────────────┘
```

---

## Three Patterns

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

## Package Overview

| Package | Description |
|---|---|
| [`dcb/`](dcb/index.md) | Low-level DCB-compliant event store backed by FoundationDB |
| [Framework root](framework/index.md) | Command runner, event reader, automation, HTTP registries |
| [`utils/`](utils/http.md) | HTTP helpers: JSON parsing, idempotency middleware |
| [`testing/`](testing/index.md) | Test utilities: `given`, `when`, `then` helpers |

---

## Development

To run tests, FoundationDB must be available. Set the build tag:

```bash
export GOFLAGS="-tags=test"
go test ./...
```
