# Append Conditions

The mechanism that makes dynamic consistency boundaries work.

---

## The Type

```go
type AppendCondition struct {
    Query Query
    After *Versionstamp
}
```

| Field | Purpose |
|---|---|
| `Query` | Events to check for conflicts |
| `After` | Position to check from (nil = entire history) |

---

## Semantics

An append with a condition succeeds **only if** no events matching `Query` exist after position `After`.

```go
err := store.Append(ctx, events, &AppendCondition{
    Query: query,
    After: &lastSeen,
})
```

| Outcome | Meaning |
|---|---|
| `nil` | Append succeeded, no conflicting events |
| `ErrAppendConditionFailed` | A matching event was written after `After` |

---

## How the Framework Uses It

You rarely construct `AppendCondition` manually. The framework handles it:

### 1. Command Reads Events

```go
ra.ReadEvents(ctx, query, handler)
```

Internally, `ReadEvents` tracks the last versionstamp seen.

### 2. Command Appends

```go
ra.AppendEvents(ctx, event)
```

Internally builds:

```go
AppendCondition{
    Query: queryFromReadEvents,
    After: &lastSeenVersionstamp,
}
```

### 3. Store Checks Condition

Before writing, the store scans for events matching `Query` after `After`. If any exist, append fails.

### 4. Runner Retries

`CommandRunner` catches `ErrAppendConditionFailed` and reruns the command from scratch.

---

## FoundationDB Transaction Isolation

The condition check and append happen in a single FDB transaction:

```
BEGIN TRANSACTION
  1. Scan indices for Query matches after After
  2. If matches exist → ABORT
  3. Write events with versionstamp
COMMIT
```

FDB's serializable isolation guarantees no race between check and write.

---

## Multiple Read Patterns

### Single Query

```go
ra.ReadEvents(ctx, query1, handler)
ra.AppendEvents(ctx, event)
// condition uses query1
```

### Multiple Queries

```go
ra.ReadEvents(ctx, query1, handler1)
ra.ReadEvents(ctx, query2, handler2)
ra.AppendEvents(ctx, event)
// condition uses query1 OR query2
```

The condition expands to cover all queries used during the command.

### No Read

```go
// No ReadEvents call
ra.AppendEvents(ctx, event)
// No condition — always succeeds
```

Useful for unconditional writes (audit logs, etc).

---

## Condition Composition

When a command calls `ReadEvents` multiple times, conditions are merged:

```go
Query{Items: []QueryItem{
    {Types: ["OrderPlaced"], Tags: ["order:123"]},     // from query1
    {Types: ["CreditLimitSet"], Tags: ["user:42"]},    // from query2
}}
```

The append fails if **any** of these event patterns appeared since the reads.

---

## Debugging Conflicts

When `ErrAppendConditionFailed` happens frequently:

1. **Check query scope** — is your query too broad?
2. **Check contention** — many commands reading same entities?
3. **Consider splitting** — can the command read less?

The framework logs retry attempts. Monitor retry rate to detect hot spots.

---

## Manual Usage

For low-level control:

```go
err := store.Append(ctx, []dcb.Event{rawEvent}, &dcb.AppendCondition{
    Query: dcb.Query{Items: []dcb.QueryItem{{Types: []string{"UserCreated"}}}},
    After: &someVersionstamp,
})
```

Prefer the framework's automatic tracking unless you need custom behavior.
