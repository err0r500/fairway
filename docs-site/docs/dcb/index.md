# DCB Store

The `dcb/` package provides low-level, DCB-compliant event storage backed by FoundationDB. It is the foundation everything else is built on.

---

## What is DCB?

[Dynamic Consistency Boundaries (DCB)](https://dcb.events) is an event sourcing model where optimistic locking is scoped not to a fixed aggregate boundary, but to the data a command actually reads. This eliminates unnecessary contention between commands that touch different parts of the system.

In Fairway, this means:

- A command reads only the events relevant to its decision.
- The conditional append checks only for new events matching that specific read.
- Two commands operating on different entities never block each other, even in the same "domain".

---

## The `DcbStore` Interface

```go
type DcbStore interface {
    Append(ctx context.Context, events []Event, condition *AppendCondition) error
    Read(ctx context.Context, query Query, opts *ReadOptions) iter.Seq2[StoredEvent, error]
    ReadAll(ctx context.Context) iter.Seq2[StoredEvent, error]
    Database() fdb.Database
    Namespace() string
}
```

| Method | Purpose |
|---|---|
| `Append` | Writes events, optionally with a conditional guard |
| `Read` | Streams events matching a query in versionstamp order |
| `ReadAll` | Streams every event in the namespace |
| `Database` | Returns the underlying FDB database handle |
| `Namespace` | Returns the namespace prefix for this store |

---

## Sections

- [Interface & Types](store.md) — All types: `Event`, `Versionstamp`, `AppendCondition`, `StoredEvent`
- [Queries](queries.md) — How to filter events with `Query` and `QueryItem`
- [Storage Layout](storage.md) — How events are indexed in FoundationDB
- [Streaming Reads](streaming.md) — K-way merge algorithm for memory-efficient streaming

---

## Creating a Store

```go
import (
    "github.com/apple/foundationdb/bindings/go/src/fdb"
    "github.com/err0r500/fairway/dcb"
)

fdb.MustAPIVersion(740)
db := fdb.MustOpenDefault()
store := dcb.NewDcbStore(db, "myapp")
```

The `namespace` parameter isolates this store from other stores sharing the same FDB cluster. Use distinct namespaces per application or environment.

### With Observability

```go
store := dcb.NewDcbStore(db, "myapp",
    dcb.StoreOptions{}.WithLogger(myLogger),
    dcb.StoreOptions{}.WithMetrics(myMetrics),
)
```

---

## Errors

| Error | Meaning |
|---|---|
| `ErrEmptyEvents` | `Append` called with an empty slice |
| `ErrAppendConditionFailed` | The append condition was violated — another writer appended a matching event first |
| `ErrInvalidQuery` | A `QueryItem` has neither types nor tags |
