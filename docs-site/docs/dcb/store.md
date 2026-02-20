# DCB Interface & Types

This page documents the core types of the `dcb` package.

---

## `Event`

The raw unit stored in the event log.

```go
type Event struct {
    Type string   // Event type name (e.g. "UserCreated")
    Tags []string // Tags for scoping and filtering (AND semantics within a query)
    Data []byte   // JSON-encoded payload
}
```

- **`Type`** identifies the event kind. It is used to route reads to the correct type index.
- **`Tags`** attach entity-scoped labels (e.g. `"list:my-list"`, `"user:42"`). Tags are sorted alphabetically at write time for consistent indexing.
- **`Data`** is an opaque JSON blob. At the framework layer, this contains the serialized `fairway.Event` envelope (timestamp + user data).

!!! note
    `dcb.Event` is the low-level representation. At the framework layer, you work with `fairway.Event` instead, which wraps a user-defined struct and a timestamp.

---

## `Versionstamp`

```go
type Versionstamp [12]byte
```

A globally unique, monotonically increasing identifier assigned by FoundationDB at commit time.

- **Bytes 0–9**: 10-byte FDB transaction version (assigned by FDB, globally ordered)
- **Bytes 10–11**: 2-byte user version (sequence number within a transaction, 0–65535)

Together they form a 12-byte value that is:

- **Globally unique** across all transactions
- **Monotonically increasing** — later commits always have larger versionstamps
- **Stable** — once assigned, never changes

### Methods

```go
// Compare returns -1, 0, or +1
func (v Versionstamp) Compare(other Versionstamp) int

// String returns the hex representation
func (v Versionstamp) String() string
```

---

## `StoredEvent`

An event as it exists in the store, enriched with its assigned position.

```go
type StoredEvent struct {
    Event               // Type, Tags, Data
    Position Versionstamp
}
```

`Position` is the versionstamp assigned at commit time. Events are always yielded in `Position` order by `Read` and `ReadAll`.

---

## `AppendCondition`

Controls whether an `Append` is allowed to proceed.

```go
type AppendCondition struct {
    Query Query
    After *Versionstamp // Optional: only check events strictly AFTER this position
}
```

The condition is satisfied — and the append proceeds — **only if** no events matching `Query` exist after position `After`.

- If `After` is `nil`, the condition checks the entire event history.
- If events matching the query exist after the given versionstamp, `Append` returns `ErrAppendConditionFailed`.

This is the DCB optimistic lock: the command declares "I read up to position X, and my decision is only valid if no relevant events have been written since."

!!! example "How the condition maps to a command lifecycle"
    1. Command calls `ReadEvents(query, handler)` — tracks last seen versionstamp.
    2. Command calls `AppendEvents(event)` — internally builds `AppendCondition{Query: query, After: lastSeen}`.
    3. If another writer appended a matching event between step 1 and step 2, `Append` fails with `ErrAppendConditionFailed`.
    4. The `CommandRunner` retries the entire command from step 1.

---

## `ReadOptions`

```go
type ReadOptions struct {
    Limit int           // Maximum events to return (0 = unlimited)
    After *Versionstamp // Only return events strictly after this position
}
```

Used with `DcbStore.Read` to paginate or resume reading from a known position.

---

## `Query` and `QueryItem`

See the [Queries page](queries.md) for full semantics and examples.

```go
type QueryItem struct {
    Types []string // OR: match any of these types
    Tags  []string // AND: must have all these tags
}

type Query struct {
    Items []QueryItem // OR: match any item
}
```

---

## Constructing the Store

```go
func NewDcbStore(db fdb.Database, namespace string, opts ...func(*fdbStore)) DcbStore
```

Options are applied via `StoreOptions`:

```go
opts := dcb.StoreOptions{}
store := dcb.NewDcbStore(db, "myapp",
    opts.WithLogger(logger),
    opts.WithMetrics(metrics),
)
```

### Observability Interfaces

```go
type Logger interface {
    // implementation-defined, used internally
}

type Metrics interface {
    // implementation-defined, used internally
}
```

Both default to no-op implementations when not provided.
