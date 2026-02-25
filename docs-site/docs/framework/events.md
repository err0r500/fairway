# Events

At the framework layer, events are plain Go structs wrapped in a `fairway.Event` envelope that adds a timestamp.

---

## `fairway.Event`

```go
type Event struct {
    OccurredAt time.Time `json:"occurredAt"`
    Data       any       `json:"data"`
}
```

- `OccurredAt` — when the event happened (set automatically by `NewEvent`)
- `Data` — the user-defined event struct

### Creating Events

```go
// With auto-generated timestamp
event := fairway.NewEvent(ListCreated{ListId: "abc", Name: "Shopping"})

// With an explicit timestamp (for migrations, replays, tests)
event := fairway.NewEventAt(ListCreated{...}, someTime)
```

---

## User Event Structs

User events are plain Go structs. They can optionally implement two interfaces:

### `Tags() []string`

Scopes the event to a specific entity. Tags are used for filtering in queries.

```go
type ListCreated struct {
    ListId string `json:"listId"`
    Name   string `json:"name"`
}

func (e ListCreated) Tags() []string {
    return []string{"list:" + e.ListId}
}
```

If a struct does not implement `Tags()`, it is stored without tags (global scope).

### `TypeString() string`

Overrides the event type name used in storage. Defaults to `reflect.TypeOf(data).Name()`.

```go
func (e ListCreated) TypeString() string {
    return "list.created.v1"
}
```

Use this when you want a stable type name that does not depend on the Go struct name.

---

## Serialization

`fairway.Event` is serialized to JSON with this envelope structure:

```json
{
  "occurredAt": "2024-01-15T10:30:00Z",
  "data": {
    "listId": "abc",
    "name": "Shopping"
  }
}
```

This JSON blob becomes the `Data` field of the underlying `dcb.Event`.

The type name (`ListCreated` by default, or the result of `TypeString()`) is stored separately as the `dcb.Event.Type` field and used for indexing and deserialization.

---

## Deserialization

When reading events back from the store, the framework:

1. Looks up the type name in its internal registry (populated by `QueryItem.Types(...)`)
2. Unmarshals the JSON envelope to recover `OccurredAt`
3. Unmarshals the inner `data` field into a new instance of the registered Go type
4. Returns a `fairway.Event{OccurredAt: ..., Data: <concrete struct>}`

The handler receives a typed `fairway.Event` with `Data` already cast to the correct concrete type. Use a type switch:

```go
func(e fairway.Event) bool {
    switch data := e.Data.(type) {
    case ListCreated:
        // data.ListId, data.Name are available
    case ItemAdded:
        // data.ItemId is available
    }
    return true
}
```

---

## Converting to `dcb.Event`

If you need to work at the DCB layer directly:

```go
dcbEvent, err := fairway.ToDcbEvent(fairwayEvent)
```

This serializes the `fairway.Event` to JSON and populates `dcb.Event.Type`, `dcb.Event.Tags`, and `dcb.Event.Data`.
