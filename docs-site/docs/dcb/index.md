# DCB Store

The `dcb/` package provides the event storage layer. It implements [Dynamic Consistency Boundaries](https://dcb.events) on top of FoundationDB.

---

## What DCB Solves

Traditional event stores fix consistency at the stream level. One stream = one aggregate = one lock.

DCB removes this constraint. Consistency boundaries emerge from what each command actually reads.

**[Learn more: Dynamic consistency →](../solution/dynamic-consistency.md)**

---

## The Interface

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
| `Append` | Write events, optionally with conditional guard |
| `Read` | Stream events matching a query |
| `ReadAll` | Stream all events in namespace |

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

### With Observability

```go
store := dcb.NewDcbStore(db, "myapp",
    dcb.StoreOptions{}.WithLogger(myLogger),
    dcb.StoreOptions{}.WithMetrics(myMetrics),
)
```

---

## Sections

- [Interface & Types](store.md) — `Event`, `Versionstamp`, `StoredEvent`
- [Append Conditions](append-conditions.md) — conditional writes, conflict detection
- [Storage Layout](storage.md) — how events are indexed in FDB
- [Streaming Reads](streaming.md) — k-way merge for memory-efficient reads

---

## Errors

| Error | Meaning |
|---|---|
| `ErrEmptyEvents` | `Append` called with empty slice |
| `ErrAppendConditionFailed` | Condition violated — retry |
| `ErrInvalidQuery` | `QueryItem` has neither types nor tags |
