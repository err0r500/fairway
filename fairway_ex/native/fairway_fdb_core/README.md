# fairway_fdb_core

Shared Rust library containing the complete DCB store implementation.
Consumed as a dependency by both `fairway_fdb` (Elixir NIF) and
`fairway_fdb_c` (C FFI for Go). No NIF or C boilerplate lives here — only
the algorithms that must run inside FDB transactions.

---

## Source files

### `keys.rs` — FDB key encoding

Encodes and decodes every key used in the three FDB subspaces. All keys use
FDB's tuple layer encoding, matching the Go `fdb/tuple` package byte-for-byte
so the two implementations are storage-compatible.

Key functions:

| Function | Purpose |
|---|---|
| `events_prefix(ns)` | Packed prefix for the primary events subspace |
| `event_key_incomplete(ns, batch_idx)` | Primary event key with an incomplete versionstamp at `batch_idx` |
| `type_index_key_incomplete(ns, type, batch_idx)` | Type-index key with incomplete versionstamp |
| `tag_index_key_incomplete(ns, tags, type, batch_idx)` | Tag-tree key with incomplete versionstamp |
| `range_after_versionstamp(prefix, vs)` | `(begin, end)` range starting strictly after `vs` within `prefix` |
| `prefix_range(prefix)` | `(begin, end)` range for the entire `prefix` subspace |
| `encode_event_value(type, tags, data)` | Pack event data as `tuple(type, tags_tuple, data_bytes)` |
| `decode_event_value(bytes)` | Unpack the above |
| `extract_versionstamp_from_key(key)` | Pull the trailing versionstamp element from any index key |
| `extract_type_from_tag_key(key)` | Pull the type string (second-to-last element) from a tag-tree key |
| `bytes_to_versionstamp(b)` | Convert raw 12-byte slice → `Versionstamp` |
| `versionstamp_to_bytes(vs)` | Convert `Versionstamp` → 12-byte `Vec<u8>` |

**Incomplete versionstamps** — FDB's `SetVersionstampedKey` mutation takes a
key containing a placeholder at a known byte offset and atomically replaces it
with the commit versionstamp. This is how all three subspace entries for a
single event get the same versionstamp even though they are written as separate
`atomic_op` calls in the same transaction.

### `tags.rs` — tag subset generation

```rust
pub fn generate_all_subsets(tags: &[String]) -> Vec<Vec<String>>
```

Generates every non-empty subset of the input tags, sorted alphabetically
within each subset, in bit-mask enumeration order. For `n` tags this produces
`2ⁿ − 1` subsets.

**Why all subsets?** The tag-tree index is designed so that querying events
with a specific set of tags is a single FDB range scan with no post-filtering.
To make this work, every possible tag combination that could appear in a query
must have an index entry. Writing all subsets at append time is the price of
fast arbitrary-subset reads.

For events with few tags (the typical case is 1–3 tags) this is cheap:
- 1 tag → 1 subset
- 2 tags → 3 subsets
- 3 tags → 7 subsets
- 4 tags → 15 subsets

### `read.rs` — event reads with k-way merge

#### `read_events(db, namespace, query_items, opts)`

Executes inside a single `db.create_trx()` / implicit `ReadTransact`. Steps:

1. For each `QueryItem`, calls `build_query_ranges` to get a list of `(begin, end)`
   FDB key ranges.
2. Fetches all keys in each range via `trx.get_range()`.
3. Runs a **k-way merge** over all result sets using a binary heap, producing
   events in versionstamp order (or reverse order if `opts.reverse` is set).
4. Deduplicates by versionstamp (a single event can appear in multiple index
   ranges if it matches multiple query items).
5. For each unique versionstamp, fetches the full event data from the primary
   events subspace (`{ns}/e/{vs}`).

#### `build_query_ranges`

Three dispatch paths:

| Query shape | Index used | Ranges produced |
|---|---|---|
| Types only (no tags) | Type index `{ns}/t/{type}/{vs}` | One range per type |
| Types + tags | Tag-tree `{ns}/g/{sorted_tags}/_e/{type}/{vs}` | One range per type |
| Tags only | Tag-tree (type discovery first) | One range per discovered type |

The **tags-only** path first scans the `_e` marker subspace to discover which
event types have ever been tagged with the given tag combination, then builds
one range per discovered type. This avoids a full-table scan while still
allowing queries that don't specify a type.

#### K-way merge

The merge uses a `BinaryHeap` of `(versionstamp_bytes, range_index, key_index)`
tuples. For forward reads the heap is a min-heap (`Reverse<Vec<u8>>`); for
reverse reads it is a max-heap (natural `Vec<u8>` ordering, since byte
comparison of versionstamps is lexicographic = correct order).

This mirrors Go's `vsHeap` implementation in `dcb/read.go` exactly.

#### `read_all_events(db, namespace)`

Scans the entire primary events subspace (`{ns}/e/`) in one range read.
Used for full projections and test helpers. Decodes each value in-place
without touching any index.

### `append.rs` — atomic DCB append

#### `append_events(db, namespace, events, conditions)`

All work happens inside a single `db.create_trx()` + `trx.commit()`:

```
for each condition:
    for each query_item in condition.query_items:
        scan the relevant index range (LIMIT 1) after after_position
        if any key found → return Err(ConditionFailed)  ← abort, no commit

for each event (batch_index 0, 1, 2, ...):
    write primary key:  {ns}/e/{incomplete_vs(batch_index)}  → encoded value
    write type index:   {ns}/t/{type}/{incomplete_vs}        → ""
    for each tag subset:
        write tag-tree: {ns}/g/{subset}/_e/{type}/{incomplete_vs} → ""
```

On `trx.commit()`, FDB atomically:
- Verifies that no key in any range read during the transaction changed since
  the transaction's read version (snapshot isolation conflict detection).
- Fills in all incomplete versionstamps with the commit versionstamp.

If FDB detects a conflict (another transaction committed to a range we read),
it returns an error that surfaces as `{:error, :condition_failed}` in Elixir /
`ErrAppendConditionFailed` in Go. The command runner retries from scratch.

---

## Wire types

All public types derive `serde::Serialize + serde::Deserialize` for JSON
transport. This is how both the NIF (Elixir) and the C FFI (Go) communicate
with the core without a custom binary codec.

```rust
pub struct QueryItem {
    pub types: Vec<String>,   // OR semantics within item
    pub tags: Vec<String>,    // AND semantics within item
}

pub struct ReadOpts {
    pub limit: Option<u32>,
    pub after_position: Option<String>,  // hex24: "aabbcc..."
    pub reverse: bool,
}

pub struct StoredEvent {
    pub position: String,    // hex24
    pub event_type: String,
    pub tags: Vec<String>,
    pub data_b64: String,    // base64-encoded application bytes
}

pub struct EventToAppend {
    pub event_type: String,
    pub tags: Vec<String>,
    pub data_b64: String,
}

pub struct AppendCondition {
    pub query_items: Vec<QueryItem>,
    pub after_position: Option<String>,  // hex24 or null
}
```

**Positions as hex strings** — raw 12-byte versionstamps are encoded as
24-character lowercase hex strings. This is safe to embed in JSON, trivially
comparable (lexicographic order on the hex string equals versionstamp order),
and easy to handle in both Go (`encoding/hex`) and Elixir (`Base.decode16!`).

**Data as base64** — event data is arbitrary bytes (typically JSON produced by
the application layer). Base64 is the standard way to embed binary data in
JSON without ambiguity.

---

## Dependencies

| Crate | Reason |
|---|---|
| `foundationdb` | FDB async client with tuple layer support |
| `tokio` | Async runtime (see [workspace README](../README.md#why-tokio)) |
| `serde` + `serde_json` | JSON wire format for NIF and C FFI |
| `base64` | Encode/decode event data bytes |
| `once_cell` | One-time initialisation of the Tokio runtime |
