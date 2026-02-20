# Queries

The `fairway` package provides a fluent, type-safe query builder on top of `dcb.Query`.

---

## `QueryItem` Builder

```go
type QueryItem struct { /* unexported fields */ }
```

Build a `QueryItem` using the `NewQueryItem()` constructor and its chainable methods:

```go
item := fairway.NewQueryItem().
    Types(ListCreated{}, ItemAdded{}).
    Tags("list:" + listId)
```

### Methods

#### `Types(events ...any) QueryItem`

Adds event types to match with **OR** semantics. Pass zero-value instances of your event structs:

```go
item.Types(ListCreated{}, ItemAdded{})
// Matches ListCreated OR ItemAdded
```

`Types` uses reflection to extract the type name and stores the Go type for automatic deserialization when reading. You never need to register types manually.

#### `Tags(tags ...string) QueryItem`

Adds required tags with **AND** semantics:

```go
item.Tags("list:" + listId, "region:eu")
// Matches events that have BOTH tags
```

---

## `Query`

A `Query` is a union of `QueryItem`s with **OR** semantics between them:

```go
func QueryItems(items ...QueryItem) Query
```

```go
query := fairway.QueryItems(
    fairway.NewQueryItem().Types(ListCreated{}).Tags("list:" + listId),
    fairway.NewQueryItem().Types(ListDeleted{}).Tags("list:" + listId),
)
// Matches ListCreated OR ListDeleted for this listId
```

---

## `HandlerFunc`

The function called for each matching event:

```go
type HandlerFunc func(Event) bool
```

Return `true` to continue iterating, `false` to stop early.

```go
var found bool
query := fairway.QueryItems(
    fairway.NewQueryItem().Types(ListCreated{}).Tags("list:" + listId),
)

ra.ReadEvents(ctx, query, func(e fairway.Event) bool {
    found = true
    return false // stop after first match
})
```

---

## Semantics Summary

| Scope | Operator | Example |
|---|---|---|
| Multiple types in one item | **OR** | `Types(A{}, B{})` — match A or B |
| Multiple tags in one item | **AND** | `Tags("t1", "t2")` — must have both |
| Multiple items in a query | **OR** | `QueryItems(item1, item2)` — match either |

---

## Type Registration

When you call `.Types(SomeEvent{})`, the `QueryItem` stores:

- The type name string (`"SomeEvent"` or `TypeString()` result)
- The `reflect.Type` of `SomeEvent`

When events are read back, the reader uses this registry to deserialize the JSON payload into the correct Go struct. **If you read events of a type you did not register via `Types(...)`, deserialization will fail.**

---

## Example: Combining Types and Tags

```go
// All events for a specific list
query := fairway.QueryItems(
    fairway.NewQueryItem().
        Types(ListCreated{}, ItemAdded{}, ItemRemoved{}, ListArchived{}).
        Tags("list:" + listId),
)

// Both list-scoped and global events
query := fairway.QueryItems(
    fairway.NewQueryItem().Types(ListCreated{}).Tags("list:" + listId),
    fairway.NewQueryItem().Types(SystemEvent{}), // no tags = global
)
```
