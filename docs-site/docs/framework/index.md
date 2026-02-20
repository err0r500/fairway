# Framework Layer

The `fairway` root package builds on top of the [DCB store](../dcb/index.md) to provide high-level abstractions for building event-sourced applications.

---

## Philosophy: Micromodules

The UNIX philosophy applied to backends: system behavior emerges from tiny, independent modules composed through a shared event log.

Each module:

- **Does one thing** — changes one small part of the system, or displays one slice of information
- **Is disposable** — replaceable at any time without breakage or migration
- **Owns minimal state** — only what its specific task requires
- **Never talks to other modules** — all composition happens through the shared log

---

## Layer Map

```
fairway root package
├── Event           — Wrapper around user data with timestamp
├── Query           — High-level query builder (type-safe)
├── QueryItem       — Single filter clause with builder API
├── Command         — Read-then-conditionally-append pattern
├── CommandRunner   — Executes commands with automatic retry
├── EventsReader    — Reads events for projections (views)
├── Automation      — Background event-driven command execution
├── HttpChangeRegistry  — HTTP routing for commands
└── HttpViewRegistry    — HTTP routing for views
```

---

## The Three Patterns

### Command

Reads events to make a decision, then appends new events.

```
HTTP request → Command.Run() → ReadEvents() → AppendEvents() → HTTP response
                     ↑____________ErrAppendConditionFailed (retry)
```

### View

Reads events and builds a live projection, returned directly.

```
HTTP request → EventsReader.ReadEvents() → project state → HTTP response
```

### Automation

Watches the event log and runs commands without user interaction.

```
Event appended → Automation detects it → Command.Run() → new events appended
```

---

## Sections

- [Events](events.md) — `fairway.Event`, serialization, type resolution
- [Queries](queries.md) — High-level `Query` and `QueryItem` builder
- [Commands](commands.md) — `Command`, `CommandRunner`, `CommandWithEffect`, retry
- [Views](views.md) — `EventsReader`, live projections
- [Automations](automations.md) — Background workers, queues, DLQ
- [HTTP Layer](http.md) — `HttpChangeRegistry`, `HttpViewRegistry`
