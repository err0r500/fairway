# Dynamic Consistency Boundaries

Traditional event sourcing fixes consistency boundaries at design time. DCB lets boundaries emerge from what each command actually reads.

---

## The Traditional Model

Stream = consistency boundary. One aggregate per stream.

```
"Place Order" command

1. Read Order-123 stream → decide
2. Append to Order-123 stream → optimistic lock on that stream

Concurrent write to Order-123? → Conflict, retry.
Concurrent write to User-42?   → No conflict.
```

Works great when commands touch one aggregate. Breaks when they don't.

**"Place order but check credit limit":**

- Reads Order-123 stream
- Reads User-42 stream (credit limit)
- Appends to Order-123 stream

Now you need cross-stream coordination. Sagas, process managers, eventual consistency.

---

## The DCB Model

Boundary = what this command actually reads.

```
"Place Order" command

1. Read: OrderPlaced events for order-123
         CreditLimitSet events for user-42

2. Decide: is credit available?

3. Append: OrderPlaced, condition: "only if no new OrderPlaced for order-123
                                    or CreditLimitSet for user-42 since my read"
```

No streams. No aggregates. Just: "I read X, append if X hasn't changed."

---

## How It Works

### 1. Track What You Read

The command reads events via a query:

```go
ra.ReadEvents(ctx,
    fairway.QueryItems(
        fairway.NewQueryItem().Types(OrderPlaced{}).Tags("order:123"),
        fairway.NewQueryItem().Types(CreditLimitSet{}).Tags("user:42"),
    ),
    func(e fairway.Event) bool {
        // process event
        return true
    })
```

The framework tracks the last versionstamp seen.

### 2. Conditional Append

When the command appends:

```go
ra.AppendEvents(ctx, fairway.NewEvent(OrderPlaced{...}))
```

The framework builds an `AppendCondition`:

```go
AppendCondition{
    Query: sameQueryUsedForRead,
    After: lastVersionstampSeen,
}
```

### 3. Conflict Detection

The append succeeds only if no matching events were written after `lastVersionstampSeen`.

```
Timeline:
─────────────────────────────────────────────────►
     │                    │              │
     ▼                    ▼              ▼
  Read                 Append        Another write
  (v=100)              (check: v>100?) (v=101)
                           │
                           └── FAIL: CreditLimitSet@v101 matches query
```

### 4. Retry

On conflict, the `CommandRunner` retries the entire command. Fresh read, new decision, new append.

---

## What This Enables

### Commands Can Read Anything

No aggregate boundaries. A command can read:

- Events from "User" entity
- Events from "Order" entity
- Events from "Inventory" entity

All in one atomic operation. Consistency boundary = union of what it read.

### No Cross-Aggregate Coordination

No sagas. No process managers. No eventual consistency hacks. The command either succeeds atomically or retries.

### Minimal Contention

Concurrent commands conflict only if they read overlapping data.

```
Command A: reads user-42 events
Command B: reads user-99 events

→ No conflict. Execute in parallel.
```

### Refactoring Without Migration

Changed what a command needs to read? Just change the query. No stream redesign. No event migration.

---

## The Cost

**More retries under high contention**

If many commands read the same events, conflicts increase. Mitigate with narrower queries.

**No built-in snapshots**

Each command replays from the log. For hot paths, consider [views](../framework/views.md) or external caching.

---

## Implementation Details

**[See append conditions →](../dcb/append-conditions.md)**
