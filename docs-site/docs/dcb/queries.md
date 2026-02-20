# DCB Queries

Queries define which events to retrieve from the store. They are composed of one or more `QueryItem`s.

---

## Semantics

| Level | Operator | Meaning |
|---|---|---|
| Within `QueryItem.Types` | **OR** | Match events of *any* listed type |
| Within `QueryItem.Tags` | **AND** | Match events that have *all* listed tags |
| Between `Query.Items` | **OR** | Match events satisfying *any* item |

---

## `QueryItem`

```go
type QueryItem struct {
    Types []string // OR: any of these event types
    Tags  []string // AND: must have all these tags
}
```

A `QueryItem` describes a single filter clause. An event matches the item if:

- Its type is in `Types` (OR across types), **and**
- It has every tag in `Tags` (AND across tags)

At least one of `Types` or `Tags` must be non-empty.

### Combinations

| Types | Tags | Behaviour |
|---|---|---|
| non-empty | empty | Match any event of any listed type |
| empty | non-empty | Match any event tagged with all listed tags (any type) |
| non-empty | non-empty | Match events of a listed type AND tagged with all listed tags |
| empty | empty | **Invalid** — returns `ErrInvalidQuery` |

---

## `Query`

```go
type Query struct {
    Items []QueryItem // OR: any item
}
```

A `Query` is a union of `QueryItem`s. An event matches the query if it matches **any** of the items.

---

## Examples

=== "Single type"

    ```go
    query := dcb.Query{
        Items: []dcb.QueryItem{
            {Types: []string{"UserCreated"}},
        },
    }
    ```
    Matches all `UserCreated` events.

=== "Multiple types (OR)"

    ```go
    query := dcb.Query{
        Items: []dcb.QueryItem{
            {Types: []string{"UserCreated", "UserUpdated"}},
        },
    }
    ```
    Matches `UserCreated` **or** `UserUpdated`.

=== "Type + tags"

    ```go
    query := dcb.Query{
        Items: []dcb.QueryItem{
            {
                Types: []string{"OrderPlaced"},
                Tags:  []string{"tenant:acme", "region:eu"},
            },
        },
    }
    ```
    Matches `OrderPlaced` events tagged with both `tenant:acme` **and** `region:eu`.

=== "Multiple items (OR)"

    ```go
    query := dcb.Query{
        Items: []dcb.QueryItem{
            {Types: []string{"UserCreated"}, Tags: []string{"tenant:acme"}},
            {Types: []string{"TenantDeleted"}},
        },
    }
    ```
    Matches `UserCreated` for tenant acme **or** any `TenantDeleted`.

---

## Using Queries with `Read`

```go
for storedEvent, err := range store.Read(ctx, query, nil) {
    if err != nil {
        return err
    }
    fmt.Println(storedEvent.Position, storedEvent.Type)
}
```

### With `ReadOptions`

```go
after := someVersionstamp
opts := &dcb.ReadOptions{
    After: &after,
    Limit: 100,
}

for storedEvent, err := range store.Read(ctx, query, opts) {
    // ...
}
```

---

## Using Queries with `AppendCondition`

The condition says: "abort the append if any event matching this query was written after `After`."

```go
err := store.Append(ctx, events, &dcb.AppendCondition{
    Query: query,
    After: &lastSeenVersionstamp,
})
// err == dcb.ErrAppendConditionFailed → retry
```

!!! tip
    At the framework layer, `AppendCondition` is built automatically by `EventReadAppender.AppendEvents`. You rarely construct it manually.
