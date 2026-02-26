# Where Coupling Hides

Slices look independent in the architecture diagram. At runtime, coupling sneaks back in.

---

## Shared Tables

The database is the truth. Multiple slices query the same tables.

| Symptom | Cause |
|---|---|
| Merge conflicts on migrations | Two features touch the same schema |
| "My query got slow" | Another feature's index change |
| Deployment ordering | Foreign key dependencies |
| Phantom reads | Concurrent updates across slices |

The table is the coupling point. Every feature reading `users` is coupled to every feature writing it.

---

## Shared Streams

In traditional event sourcing, stream = consistency boundary.

**Problem 1: Stream design locks in coupling**

```
# Stream per entity type (User stream)
User-42: [UserCreated, AddressUpdated, CreditLimitSet, ...]

# Command needs to check credit across users?
# Now you need cross-stream coordination
```

**Problem 2: All readers of a stream are coupled**

```
User-42 stream
     │
     ├─── Order slice (reads credit limit)
     ├─── Notification slice (reads email)
     └─── Analytics slice (reads all)

# Schema change to UserCreated affects all three
```

**Problem 3: Performance coupling**

Hot streams (high-traffic entities) become bottlenecks. All commands on User-42 serialize through that stream's lock.

---

## Shared Domain Models

The "DRY" reflex creates coupling:

```go
// shared/domain/user.go
type User struct {
    ID          string
    Email       string
    CreditLimit int  // ← Added by feature A
    Preferences Prefs // ← Added by feature B
}
```

Now features A and B share code. Merge conflicts. Version coordination. "Improvements" that break other slices.

---

## Shared Event Schemas

```go
// events/user.go
type UserCreated struct {
    ID    string
    Email string
    Name  string // ← feature A adds this
}
```

Every consumer must handle `Name`. Schema evolution requires coordination.

---

## The Pattern

Coupling appears wherever multiple slices reference the same:

- Table
- Stream
- Type definition
- Schema version

**True independence means: no shared references.**

---

## The Solution

What if slices shared only one thing: the contract of what events exist?

- No shared tables
- No shared streams
- No shared types
- Just: "event X happened, here's its shape"

**[See how events-as-contracts enables this →](../solution/events-as-contracts.md)**
