# given — Setup

The `testing/given` package provides helpers for setting up test state: an isolated store, a wired HTTP server, and fixture events.

```go
import "github.com/err0r500/fairway/testing/given"
```

---

## `FreshSetup`

Creates an isolated test store, wires a command or view handler to an HTTP test server, and returns everything needed to write an end-to-end test.

```go
func FreshSetup(t *testing.T, registerFn any) (dcb.DcbStore, *httptest.Server, *resty.Client)
```

### Parameters

- `t` — the test instance (cleanup is registered automatically)
- `registerFn` — a function that accepts either `*fairway.HttpChangeRegistry` or `*fairway.HttpViewRegistry` and registers routes on it

### Returns

| Value | Type | Description |
|---|---|---|
| `store` | `dcb.DcbStore` | Isolated test store (unique namespace per test) |
| `server` | `*httptest.Server` | Running HTTP test server |
| `client` | `*resty.Client` | Pre-configured HTTP client pointed at the server |

### How It Works

1. Creates an isolated store via `SetupTestStore`
2. Creates a `CommandRunner` backed by the store
3. Instantiates the appropriate registry based on the parameter type of `registerFn`
4. Calls `registerFn` to populate the registry
5. Calls `RegisterRoutes` on the mux
6. Starts an `httptest.Server`
7. Registers cleanup to close the server and client after the test

### Example

```go
func TestCreateList(t *testing.T) {
    store, server, client := given.FreshSetup(t, Register)
    // Register is: func Register(r *fairway.HttpChangeRegistry) { ... }

    resp, err := client.R().
        SetBody(map[string]string{"name": "Shopping"}).
        Post(server.URL + "/api/lists/my-list")

    assert.NoError(t, err)
    assert.Equal(t, 201, resp.StatusCode())
}
```

### For View Tests

```go
func TestShowList(t *testing.T) {
    store, server, client := given.FreshSetup(t, Register)
    // Register is: func Register(r *fairway.HttpViewRegistry) { ... }

    // Pre-populate the store
    given.EventsInStore(store,
        fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
    )

    resp, err := client.R().Get(server.URL + "/api/lists/my-list")
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode())
}
```

---

## `SetupTestStore`

Creates an isolated, ephemeral test store.

```go
func SetupTestStore(t *testing.T) dcb.DcbStore
```

- Connects to FoundationDB (must be running)
- Uses a unique namespace per test (UUID-based)
- Registers a cleanup function that deletes the namespace after the test

```go
store := given.SetupTestStore(t)
```

!!! note
    Requires FoundationDB to be running and `GOFLAGS="-tags=test"` to be set.

---

## `EventsInStore`

Appends events to the store without any condition — for test fixtures.

```go
func EventsInStore(store dcb.DcbStore, e fairway.Event, ee ...fairway.Event)
```

Panics on error (appropriate for test setup code).

```go
given.EventsInStore(store,
    fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
    fairway.NewEvent(ItemAdded{ListId: "my-list", ItemId: "item-1", Text: "Milk"}),
)
```

Use this in view tests to pre-populate the store before issuing an HTTP request.
