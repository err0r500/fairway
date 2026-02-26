# Framework Layer

The `fairway` root package provides high-level abstractions built on FoundationDB:

- **Events** via [DCB store](../dcb/index.md) — append-only log with dynamic consistency
- **Queues** — job scheduling for automations, leases, DLQ
- **KV read models** — persistent projections for fast lookups

---

## Layer Map

```
fairway root package
├── Event             — Wrapper around user data with timestamp
├── Query / QueryItem — Filter events by type and tags
├── Command           — Read-then-conditionally-append
├── CommandRunner     — Execute with automatic retry
├── EventsReader      — Read events for projections
├── Automation        — Background event-driven execution
├── HttpChangeRegistry    — HTTP routing for commands
└── HttpViewRegistry      — HTTP routing for views
```

---

## The Three Patterns

### Command

Read events, decide, append new events.

```
HTTP request → Command.Run() → ReadEvents() → AppendEvents() → HTTP response
                     ↑____________ErrAppendConditionFailed (retry)
```

### View

Read events, build projection, return.

```
HTTP request → EventsReader.ReadEvents() → project state → HTTP response
```

### Automation

Watch event log, run commands without user.

```
Event appended → Automation detects → Command.Run() → new events
```

---

## Sections

- [Project Structure](structure.md) — file layout, `init()` registration, code generation
- [Events](events.md) — `fairway.Event`, serialization
- [Queries](queries.md) — `Query`, `QueryItem`, filtering events
- [Commands](commands.md) — `Command`, `CommandRunner`, retry
- [Views](views.md) — `EventsReader`, live projections
- [Automations](automations.md) — background workers, queues
- [HTTP Layer](http.md) — `HttpChangeRegistry`, `HttpViewRegistry`
