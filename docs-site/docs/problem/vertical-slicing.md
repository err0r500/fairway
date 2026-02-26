# The Vertical Slicing Illusion

Vertical slicing promises independent feature development. Reality is different.

---

## The Promise

Each feature is a slice: complete ownership from UI to database. No coordination. No merge conflicts. Ship whenever.

```
Feature A       Feature B       Feature C
┌─────┐         ┌─────┐         ┌─────┐
│ UI  │         │ UI  │         │ UI  │
├─────┤         ├─────┤         ├─────┤
│ API │         │ API │         │ API │
├─────┤         ├─────┤         ├─────┤
│ DB  │         │ DB  │         │ DB  │
└─────┘         └─────┘         └─────┘
  ↓               ↓               ↓
Independent    Independent    Independent
```

---

## The Reality: Shared Database

Most features share a database. Now slices touch the same tables.

```
Feature A       Feature B       Feature C
┌─────┐         ┌─────┐         ┌─────┐
│Slice│         │Slice│         │Slice│
└──┬──┘         └──┬──┘         └──┬──┘
   │               │               │
   └───────────────┼───────────────┘
                   │
            ┌──────▼──────┐
            │   users     │  ← shared table
            │   orders    │  ← shared table
            │   products  │  ← shared table
            └─────────────┘
```

Result:

- Schema changes require coordination
- Migrations affect multiple slices
- Performance tuning couples features
- "My index broke your query"

---

## The Reality: Event Store with Streams

Event sourcing is supposed to help. Put each aggregate in its own stream. Now slices are independent.

```
Feature A           Feature B           Feature C
┌──────────┐        ┌──────────┐        ┌──────────┐
│ Order-1  │        │ User-42  │        │ Product-7│
│  stream  │        │  stream  │        │  stream  │
└──────────┘        └──────────┘        └──────────┘
     ↓                   ↓                   ↓
 Independent        Independent         Independent
```

But commands rarely operate on a single aggregate.

**"Place order" needs:**

- Check inventory (Product aggregate)
- Check credit limit (User aggregate)
- Create order (Order aggregate)

Now streams must be coordinated. Options:

1. **Saga** — complex choreography, eventual consistency
2. **Process manager** — centralized coordinator, single point of failure
3. **Read across streams** — consistency boundary expands to all touched streams

The "slice" now spans multiple aggregates. The boundary you drew at design time doesn't match runtime reality.

---

## The Core Problem

**Consistency boundaries are fixed at design time.**

In a relational DB, the boundary is the transaction scope (usually "all tables you touch").

In a stream-per-aggregate event store, the boundary is the aggregate root.

Both force you to decide upfront how commands will partition data. When real commands cross those boundaries, you either:

1. Accept eventual consistency (complexity, failure modes)
2. Redesign aggregates (migrations, breaking changes)
3. Coordinate at deploy time (goodbye independence)

---

## What We Actually Need

Consistency boundaries that emerge from what each command actually reads — not from architectural diagrams drawn before the first line of code.

**[See how DCB solves this →](../solution/dynamic-consistency.md)**
