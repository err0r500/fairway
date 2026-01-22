```
matchesQuery e q ↔ ∃ wb ∈ writeBuckets e, ∃ rt ∈ readTargets q, wb = rt.bucket
```

Context

DCB (Dynamic Consistency Boundaries) uses FoundationDB to store events with:

- Type: event type (e.g., "OrderPlaced")
- Tags: set of tags (e.g., {"tenant:acme", "priority:high"})
- Version: monotonic versionstamp

Index Structure

Type Index: /t/<type>/<versionstamp>

Tag Index: /g/<sorted-tags>/\_events/<type>/<versionstamp>

Key properties:

- One entry for each non-empty subset of event tags
- Tags sorted alphabetically for consistent paths
- Versionstamp as suffix → entries sorted by version within each path

Query Semantics

A Query has:

- items: set of query items (OR'd)
- afterVersion: only match events with version > afterVersion

Each QueryItem has:

- types: set of types (event must match one)
- tags: set of tags (event must have ALL)

Event E matches Query Q iff:

- E.version > Q.afterVersion
- AND exists item I in Q.items where:
  - I.types = ∅ OR E.type ∈ I.types
  - I.tags ⊆ E.tags

Range Reads (how afterVersion is used)

Since entries are sorted by version (versionstamp suffix), queries read ranges starting at afterVersion:
┌────────────────────────┬────────────────────────────────────────────────────┐
│ QueryItem │ Range Read │
├────────────────────────┼────────────────────────────────────────────────────┤
│ types={T1,T2}, tags=∅ │ /t/T1/[afterVersion+1..], /t/T2/[afterVersion+1..] │
├────────────────────────┼────────────────────────────────────────────────────┤
│ types=∅, tags={A,B} │ /g/A/B/\_events/[afterVersion+1..] │
├────────────────────────┼────────────────────────────────────────────────────┤
│ types={T1}, tags={A,B} │ /g/A/B/\_events/T1/[afterVersion+1..] │
└────────────────────────┴────────────────────────────────────────────────────┘
What We Prove

Theorem 1 (Completeness): matchesQuery(E, Q) → conflict_detected

If another transaction appends event E that matches query Q, FDB will detect a transaction conflict.

Theorem 2 (Precision): conflict_detected → matchesQuery(E, Q)

If FDB detects a conflict, the event actually matches the query (no false positives).

Combined:
matchesQuery(E, Q) ↔ writeKeys(E) ∩ readRanges(Q) ≠ ∅

Why It Works

1. Tag subset indexing: appending event with tags {A,B,C} writes to ALL subsets → query tags {A,B} will overlap
2. Version ordering: versionstamp suffix sorts entries by version → range read [afterVersion+1..] efficient
3. Version filtering: event at version > afterVersion falls in read range → conflict detected
4. Type matching: type index or tag+type path ensures type constraint
