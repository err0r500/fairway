# then — Assertions

The `testing/then` package provides helpers for asserting outcomes in tests — specifically, what events are present in the store after an action.

```go
import "github.com/err0r500/fairway/testing/then"
```

---

## `ExpectEventsInStore`

Asserts that the store contains exactly the given events (order-independent).

```go
func ExpectEventsInStore(t *testing.T, store dcb.DcbStore, events ...fairway.Event)
```

### What it checks

- Reads all events from the store via `ReadAll`
- Compares events by **type**, **tags**, and **inner data** (the `data` field of the JSON envelope)
- Ignores the `occurredAt` timestamp (so `NewEvent(...)` can be used directly in assertions)
- Uses `assert.ElementsMatch` — order does not matter

### Example

```go
func TestCreateList(t *testing.T) {
    store, server, client := given.FreshSetup(t, Register)

    client.R().
        SetBody(map[string]string{"name": "Shopping"}).
        Post(server.URL + "/api/lists/my-list")

    then.ExpectEventsInStore(t, store,
        fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
    )
}
```

### Multiple Events

```go
then.ExpectEventsInStore(t, store,
    fairway.NewEvent(ListCreated{ListId: "l1", Name: "Shopping"}),
    fairway.NewEvent(ItemAdded{ListId: "l1", ItemId: "i1", Text: "Milk"}),
)
```

The assertion passes regardless of the order the events were appended.

### Comparison Details

Internally, each event is compared as:

```go
type eventForComparison struct {
    Type string
    Tags []string
    Data json.RawMessage  // inner data only, not the full envelope
}
```

This means:

- The type name (`"ListCreated"`) must match
- All tags must match exactly
- The JSON representation of the inner struct must match field-for-field

!!! tip
    Pass the same struct values you expect to appear. `then.ExpectEventsInStore` will serialize and compare them just as the store would.

---

## Test Pattern Summary

```go
func TestAddItem(t *testing.T) {
    // GIVEN
    store, server, client := given.FreshSetup(t, Register)
    given.EventsInStore(store,
        fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
    )

    // WHEN
    client.R().
        SetBody(map[string]string{"text": "Milk"}).
        Post(server.URL + "/api/lists/my-list/items/item-1")

    // THEN
    then.ExpectEventsInStore(t, store,
        fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
        fairway.NewEvent(ItemAdded{ListId: "my-list", ItemId: "item-1", Text: "Milk"}),
    )
}
```
