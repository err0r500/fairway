# Storage Layout

The DCB store uses a three-index architecture in FoundationDB to support efficient querying by type, by tag, and by any tag combination, while maintaining a global ordering via versionstamps.

---

## Three Indexes

### 1. Primary Event Storage

```
<namespace>/e/<versionstamp>  →  packed(type, tags[], data)
```

The primary store is the source of truth. Every event written has a single canonical entry here, keyed by versionstamp. Values are FDB tuple-encoded for type-safe serialization.

**Example:**
```
/myapp/e/\x01\x02...\x0C  →  ("UserCreated", ["tenant:acme"], <json>)
```

### 2. Type Index

```
<namespace>/t/<type>/<versionstamp>  →  nil
```

Enables fast range scans by event type. The value is always `nil` — the key itself carries all needed information.

**Example:**
```
/myapp/t/UserCreated/\x01\x02...\x0C  →  nil
/myapp/t/OrderPlaced/\x05\x06...\x0F  →  nil
```

To read all `UserCreated` events: scan the range `/myapp/t/UserCreated/*`.

### 3. Tag Tree Index

```
<namespace>/g/<tag1>/<tag2>/.../<tagN>/_e/<type>/<versionstamp>  →  nil
```

Enables fast queries on any tag combination. Tags are sorted alphabetically before being written to ensure consistent key paths regardless of insertion order.

**Example event** with tags `["tenant:acme", "priority:high"]`:

After sorting alphabetically (`priority:high` < `tenant:acme`), three index entries are created:

```
/myapp/g/priority:high/_e/OrderPlaced/<vs>          →  nil
/myapp/g/tenant:acme/_e/OrderPlaced/<vs>             →  nil
/myapp/g/priority:high/tenant:acme/_e/OrderPlaced/<vs> →  nil
```

---

## Why All Tag Subsets?

For an event with tags `[A, B, C]`, the store indexes all 2ⁿ−1 non-empty subsets:

```
[A]        [B]        [C]
[A,B]      [A,C]      [B,C]
[A,B,C]
```

This means any tag combination query is answered by a single range scan, without scanning unrelated data.

| Query | FDB range |
|---|---|
| `tags=[A]` | `/g/A/_e/*` |
| `tags=[A,B]` | `/g/A/B/_e/*` |
| `tags=[A,B,C]` | `/g/A/B/C/_e/*` |

**Trade-off:** Write amplification (2ⁿ−1 writes per event) for read efficiency. For events with a small number of tags (typically 1–3), this is very cheap.

---

## Versionstamp Mechanics

### At Write Time

```go
vs := tuple.IncompleteVersionstamp(batchIndex)
```

An *incomplete* versionstamp is a placeholder. `batchIndex` (0–65535) ensures unique ordering for multiple events in the same transaction.

### At Commit Time

FoundationDB atomically replaces all incomplete versionstamps with a real 12-byte value:

- Bytes 0–9: 10-byte monotonically increasing transaction version
- Bytes 10–11: 2-byte `batchIndex`

All three indexes (primary, type, tag) receive the **same versionstamp** within a single transaction, guaranteeing referential integrity.

---

## Query Strategy

The store maps each `QueryItem` to one or more FDB key ranges based on what is specified:

| Query type | FDB range used |
|---|---|
| Type only | `/t/<type>/*` |
| Tags only | `/g/<sorted-tags>/_e/*` (type discovered) |
| Type + tags | `/g/<sorted-tags>/_e/<type>/*` |

When a query has multiple items, each item produces its own set of ranges. All ranges are merged by the [k-way streaming algorithm](streaming.md).

### `After` Optimization

When the `After` versionstamp filter is applied:

- **Type-only** and **type+tags** queries: range start is pushed to `after+1` at the FDB level — no irrelevant data is transferred.
- **Tags-only** queries: the type string sits between `_e` and the versionstamp in the key, preventing range-level filtering. Post-filtering is applied after collection.

---

## Key Design Principles

1. **Versionstamp = global order** — monotonically increasing, assigned by FDB.
2. **Index redundancy** — trade write amplification for fast, consistent reads.
3. **Tuple encoding** — structured, type-safe binary serialization.
4. **Alphabetical tag normalization** — predictable key paths regardless of insertion order.
5. **Presence markers** — indexes store `nil` values; all information is in the key.
6. **Read transactions** — queries use read-only FDB transactions, never blocking concurrent writes.
