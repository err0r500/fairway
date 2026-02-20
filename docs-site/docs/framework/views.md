# Views

Views are the read side of Fairway. A view reads events and builds a projection on the fly — no stored state, no cache invalidation.

---

## `EventsReader`

```go
type EventsReader interface {
    ReadEvents(ctx context.Context, query Query, handler HandlerFunc) error
}
```

`ReadEvents` streams events matching the query in versionstamp order. For each event, it calls `handler(event)`. Return `true` to continue, `false` to stop early.

### Creating a Reader

```go
reader := fairway.NewReader(store)
```

The reader is stateless and safe to share across goroutines and requests.

---

## Example View

```go
func Register(registry *fairway.HttpViewRegistry) {
    registry.RegisterView("GET /api/lists/{listId}", func(reader fairway.EventsReader) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            listId := r.PathValue("listId")

            // Build projection
            list := struct {
                Id         string `json:"id"`
                Name       string `json:"name"`
                ItemsCount uint   `json:"itemsCount"`
            }{}

            err := reader.ReadEvents(r.Context(),
                fairway.QueryItems(
                    fairway.NewQueryItem().
                        Types(ListCreated{}, ItemAdded{}).
                        Tags("list:" + listId),
                ),
                func(e fairway.Event) bool {
                    switch data := e.Data.(type) {
                    case ListCreated:
                        list.Id = data.ListId
                        list.Name = data.Name
                    case ItemAdded:
                        list.ItemsCount++
                    }
                    return true // continue
                })

            if err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }

            json.NewEncoder(w).Encode(list)
        }
    })
}
```

---

## Design Properties

### No Stored State

A view projection is computed from scratch on every request by replaying events. There is no intermediate cache to invalidate or synchronise.

This makes views:

- **Always consistent** with the event log
- **Trivially evolvable** — change the projection logic and all future reads reflect the new interpretation
- **Independently deployable** — adding a new view requires no migration

### Live Streaming

Events are streamed from FoundationDB via the [k-way merge algorithm](../dcb/streaming.md). The handler is called as each event arrives, without buffering the full result set.

### Early Exit

Stop iteration early by returning `false` from the handler:

```go
var name string
reader.ReadEvents(ctx, query, func(e fairway.Event) bool {
    if data, ok := e.Data.(ListCreated); ok {
        name = data.Name
        return false // stop after finding the name
    }
    return true
})
```

---

## Event Deserialization

The reader uses the type registry built from `QueryItem.Types(...)` to deserialize events. Events whose types were not included in the query cannot be deserialized and will return an error.

Always declare every event type you want to receive in your `Types(...)` call.

---

## Wiring a View to HTTP

See the [HTTP Layer](http.md) page for how to register views with `HttpViewRegistry`.

```go
var ViewRegistry fairway.HttpViewRegistry

func init() { Register(&ViewRegistry) }
```

In `main.go`:

```go
ViewRegistry.RegisterRoutes(mux, fairway.NewReader(store))
```
