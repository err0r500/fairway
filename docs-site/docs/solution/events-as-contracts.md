# Events as Contracts

The only coupling between slices should be the events themselves.

---

## The Principle

A slice declares:

- **Input events** — what it reads
- **Output events** — what it emits

No shared code. No shared schemas. No imports across slice boundaries.

```
Slice A                     Slice B
┌──────────────────┐        ┌──────────────────┐
│ Reads:           │        │ Reads:           │
│   UserCreated    │        │   OrderPlaced    │
│                  │        │   UserCreated    │
│ Emits:           │        │                  │
│   OrderPlaced    │        │ Emits:           │
│                  │        │   InvoiceSent    │
└──────────────────┘        └──────────────────┘
         │                           │
         └───────────────────────────┘
                     │
              Shared event log
              (append-only)
```

The event log is the only integration point.

---

## Each Slice Owns Its Types

Slice A defines its own `UserCreated`:

```go
// slice_a/events.go
type UserCreated struct {
    ID    string
    Email string
}
```

Slice B defines its own `UserCreated`:

```go
// slice_b/events.go
type UserCreated struct {
    ID   string
    Name string  // different fields!
}
```

No shared import. Each slice deserializes what it needs. Unknown fields are ignored.

---

## Why This Works

**1. No compile-time coupling**

Slices don't import each other. Adding a field to an event doesn't break consumers.

**2. Independent evolution**

Slice A can add `PhoneNumber` to its local `UserCreated` definition without touching Slice B.

**3. Deploy independently**

No coordination. Slice B doesn't need to redeploy when Slice A changes.

**4. True vertical slicing**

Each slice is a standalone module. The event log is just a message bus with a contract: event type + shape.

---

## The Contract

Events are the API between slices:

| Contract element | Who defines it |
|---|---|
| Event type name | Producer |
| Required fields | Producer documents, consumers pick what they need |
| Tags (entity scope) | Producer |

Consumers are responsible for handling schema evolution (missing fields → defaults, extra fields → ignored).

---

## In Fairway

Each command defines its own local event types:

```go
// createlist/command.go
type ListCreated struct {
    ListId string
    Name   string
}

func (cmd CreateList) Run(ctx context.Context, ra fairway.EventReadAppender) error {
    // reads only what this command needs
    // emits ListCreated with its own type definition
}
```

No shared `events/` package. No domain model. Just the event contract.

---

## Tradeoffs

| Benefit | Cost |
|---|---|
| Zero compile-time coupling | Type names must match across slices |
| Independent deploys | Must document event shapes |
| Each slice picks its own fields | No compiler catching mismatches |

The benefit — true slice independence — usually outweighs the cost.

---

## Next

Events solve the coupling problem. But how do you get consistency without shared streams?

**[See dynamic consistency boundaries →](dynamic-consistency.md)**
