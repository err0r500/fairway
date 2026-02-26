# Queries

Queries define which events to read. They are used by commands, views, and automations.

---

## `QueryItem`

A single filter clause:

```go
item := fairway.NewQueryItem().
    Types(ListCreated{}, ItemAdded{}).  // OR: match any type
    Tags("list:" + listId)               // AND: must have all tags
```

| Method | Semantics |
|---|---|
| `Types(events...)` | OR — match any listed type |
| `Tags(tags...)` | AND — must have all listed tags |

Pass zero-value structs to `Types()`. The framework extracts type names and registers them for deserialization.

---

## `Query`

A union of `QueryItem`s:

```go
query := fairway.QueryItems(
    fairway.NewQueryItem().Types(ListCreated{}).Tags("list:" + listId),
    fairway.NewQueryItem().Types(ListDeleted{}).Tags("list:" + listId),
)
```

Items are combined with OR — an event matches if it satisfies **any** item.

---

## Semantics Summary

| Scope | Operator |
|---|---|
| Types within item | OR |
| Tags within item | AND |
| Items within query | OR |

---

## In Commands

Every `ReadEvents` call is tracked. When `AppendEvents` runs, all queries become part of the [append condition](../dcb/append-conditions.md):

```go
func (cmd myCommand) Run(ctx context.Context, ra fairway.EventReadAppender) error {
    ra.ReadEvents(ctx, query1, handler1)  // tracked
    ra.ReadEvents(ctx, query2, handler2)  // tracked

    // AppendEvents checks: no events matching query1 OR query2 since reads
    return ra.AppendEvents(ctx, event)
}
```

If any matching event was written between read and append, the command retries.

---

## In Views

Views use queries to filter events for projection:

```go
reader.ReadEvents(ctx,
    fairway.QueryItems(
        fairway.NewQueryItem().
            Types(ListCreated{}, ItemAdded{}).
            Tags("list:" + listId),
    ),
    func(e fairway.Event) bool {
        // project event
        return true
    })
```

---

## Type Registration

`Types()` registers Go types for deserialization. Only events whose types are registered can be deserialized:

```go
// This works — ItemAdded is registered
query := fairway.NewQueryItem().Types(ListCreated{}, ItemAdded{})

// Reading an ItemRemoved event would fail — not registered
```

Always include every event type you want to handle.
