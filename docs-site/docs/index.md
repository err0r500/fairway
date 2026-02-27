# Fairway

**Tiny, independent vertical slices. One shared event log. Zero coupling.**

## The Problem

Vertical slicing promises independent feature development. Reality usually delivers hidden coupling.

**Shared databases** — slices touch the same tables. Schema changes require coordination.

**Stream-per-aggregate** — consistency boundaries are fixed at design time. Commands that cross aggregates need sagas.

**[Read more: The vertical slicing illusion →](problem/vertical-slicing.md)**

---

## The Solution

**Events as the only contract.** Slices share nothing but the event log. No shared tables, streams, or types.

**Dynamic consistency boundaries.** Each command's boundary emerges from what it actually reads — not from architectural diagrams.

**[Read more: Dynamic consistency →](solution/dynamic-consistency.md)**

---

## Architecture

```
┌─────────────────────────────────────────┐
│         Testing Helpers                 │  given / when / then
├─────────────────────────────────────────┤
│         HTTP Layer                      │  HttpChangeRegistry / HttpViewRegistry
├─────────────────────────────────────────┤
│         Framework Layer                 │  Command / View / Automation
├─────────────────────────────────────────┤
│         FoundationDB                    │  events (DCB) / queues / KV read models
└─────────────────────────────────────────┘
```

One datastore for everything: events, job queues, read model persistence.

---

## Three Patterns

Every module implements one of these:

```
User action        Event log           Projection
    │                  │                   │
    ▼                  ▼                   ▼
┌──────────┐    ┌───────────────┐    ┌──────────┐
│ Command  │───▶│    Events     │───▶│   View   │
└──────────┘    └───────┬───────┘    └──────────┘
                        │
                ┌───────▼──────┐
                │  Automation  │  (View → Command, no user)
                └──────────────┘
```

---

## Quick Start

**Prerequisites:** Go 1.24+, FoundationDB installed.

```bash
git clone https://github.com/err0r500/fairway
cd fairway/examples/todolist
go generate ./...
go run .
```

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
| [Framework](framework/index.md) | Commands, views, automations, queues, KV read models |
| [`dcb/`](dcb/index.md) | DCB-compliant event store on FoundationDB |
| [`utils/`](utils/http.md) | HTTP helpers, idempotency middleware |
| [`testing/`](testing/index.md) | `given`, `when`, `then` test helpers |

---

## Learn More

- **[The Problem](problem/vertical-slicing.md)** — why vertical slicing fails
- **[The Solution](solution/events-as-contracts.md)** — events as contracts
- **[DCB Store](dcb/index.md)** — the foundation
- **[Framework](framework/index.md)** — commands, views, automations

---

## Development

```bash
export GOFLAGS="-tags=test"
go test ./...
```
