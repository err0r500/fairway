# Storage Architecture

## Overview

The event store uses a three-index architecture in FoundationDB to enable efficient querying by type and tags while maintaining global ordering through versionstamps.

## Data Layout

### 1. Primary Event Storage

**Key structure:**
```
<namespace>/e/<versionstamp>
```

**Value structure:**
```
Packed tuple: (type: string, tags: []string, data: []byte)
```

**Example:**
```
Key:   /myapp/e/\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0A\x0B\x0C
Value: (packed) ("UserCreated", ["tenant:acme", "priority:high"], <json_bytes>)
```

The value is a **binary blob** using FDB's tuple encoding, not a path structure. Tuple encoding provides:
- Type-safe serialization/deserialization
- Forward compatibility for schema evolution
- Consistent encoding across FDB ecosystem

### 2. Type Index

**Key structure:**
```
<namespace>/t/<type>/<versionstamp>
```

**Value:** `nil` (presence marker only)

**Purpose:** Fast queries by event type alone

**Example:**
```
/myapp/t/UserCreated/\x01\x02...\x0C → nil
/myapp/t/UserDeleted/\x05\x06...\x0F → nil
```

Query `type=UserCreated` scans the range `/myapp/t/UserCreated/*`

### 3. Tag Tree Index

**Key structure:**
```
<namespace>/g/<tag1>/<tag2>/.../<tagN>/_events/<type>/<versionstamp>
```

**Value:** `nil` (presence marker only)

**Purpose:** Enable queries on any tag combination

**Tag ordering:** Tags are sorted **alphabetically** before indexing to ensure consistent query paths regardless of insertion order.

**Example event:**
```go
Event{
    Type: "OrderPlaced",
    Tags: ["tenant:acme", "priority:high"],
    Data: ...
}
```

**Generates 3 index entries:**
```
/myapp/g/priority:high/_events/OrderPlaced/<vs> → nil
/myapp/g/tenant:acme/_events/OrderPlaced/<vs> → nil
/myapp/g/priority:high/tenant:acme/_events/OrderPlaced/<vs> → nil
```

Note: Tags are sorted (`priority:high` < `tenant:acme` alphabetically) so the combined path is `priority:high/tenant:acme`.

### Why All Subsets?

For an event with tags `[A, B, C]`, we generate indexes for all 2^n - 1 subsets:
- `[A]`, `[B]`, `[C]`
- `[A,B]`, `[A,C]`, `[B,C]`
- `[A,B,C]`

This enables efficient querying of **any tag combination** without scanning irrelevant data:
- Query `tags=[A]` → scan `/g/A/_events/*`
- Query `tags=[A,B]` → scan `/g/A/B/_events/*`
- Query `tags=[A,B,C]` → scan `/g/A/B/C/_events/*`

**Trade-off:** Write amplification (2^n - 1 writes per event) for read efficiency.

## Versionstamp Mechanics

### Incomplete vs Complete Versionstamps

**At write time (appendSingle):**
```go
vs := tuple.IncompleteVersionstamp(batchIndex)
```
- Creates placeholder versionstamp
- `batchIndex` (0-65535) ensures unique ordering within transaction
- FDB replaces placeholder with actual versionstamp at commit time

**At commit:**
- FDB assigns monotonically increasing 10-byte transaction version
- Combines with 2-byte user version (batchIndex) → 12-byte versionstamp
- **All three indexes receive identical versionstamp** atomically

### Atomicity Guarantee

Single transaction writes:
1. Primary storage: `e/<vs>`
2. Type index: `t/<type>/<vs>`
3. Tag indexes: `g/<tag_path>/<type>/<vs>` (all subsets)

All receive same `<vs>` value → referential integrity guaranteed.

## Query Strategy

### Building FDB Ranges

Queries map to FDB key ranges based on specificity:

**Query: type only**
```go
Query{Items: []QueryItem{{Types: ["UserCreated"]}}}
```
→ Range: `/t/UserCreated/*`

**Query: tags only**
```go
Query{Items: []QueryItem{{Tags: ["tenant:acme"]}}}
```
→ Range: `/g/tenant:acme/_events/*`

**Query: type + tags**
```go
Query{Items: []QueryItem{{
    Types: ["UserCreated"],
    Tags: ["tenant:acme", "priority:high"]
}}}
```
→ Range: `/g/priority:high/tenant:acme/_events/UserCreated/*`

**Query: OR semantics**
```go
Query{Items: []QueryItem{
    {Types: ["UserCreated"]},
    {Types: ["UserUpdated"]},
}}
```
→ Two ranges: `/t/UserCreated/*` and `/t/UserUpdated/*`

### Versionstamp Collection and Filtering

Read process:
1. Scan all FDB ranges for query items
2. Collect unique versionstamps in map (deduplication)
3. Apply `After` filter (method depends on query type - see below)
4. Sort versionstamps lexicographically
5. Apply limit
6. Fetch event data from primary storage: `events/<vs>`

### After Filtering Optimization

The `After` versionstamp filter is applied at different levels depending on query structure:

**FDB Range-Level Filtering (Optimized)**

When versionstamp is the **last element** in the key, we can filter at the FDB range level:

1. **Type-only queries**: `/t/<type>/<versionstamp>`
   - Versionstamp is last → create range starting after specified versionstamp
   - FDB skips irrelevant data entirely

2. **Tags + type queries**: `/g/<tags>/_events/<type>/<versionstamp>`
   - Versionstamp is last → create range starting after specified versionstamp
   - FDB skips irrelevant data entirely

**Application-Level Filtering (Required)**

When versionstamp is **not last** in the key:

3. **Tags without type**: `/g/<tags>/_events/<type>/<versionstamp>`
   - Type string comes between `_events` and versionstamp
   - Cannot use byte-range filtering (type breaks ordering)
   - Must post-filter after collecting versionstamps

This optimization reduces data transfer and improves performance for the common cases (type-only and tags+type queries) while maintaining correctness for all query patterns.

## Key Design Principles

1. **Versionstamp = Global Order**: Monotonically increasing, globally unique identifier
2. **Index Redundancy**: Trade write cost for read efficiency
3. **Tuple Encoding**: Structured, type-safe value serialization
4. **Alphabetical Ordering**: Predictable query paths independent of insertion order
5. **Presence Markers**: Indexes store `nil` values (keys provide all needed info)
6. **Read Transactions**: Queries use read-only transactions for consistency without blocking writes
