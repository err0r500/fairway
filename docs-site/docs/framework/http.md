# HTTP Layer

Fairway provides two registries for wiring commands and views to HTTP routes: `HttpChangeRegistry` for commands and `HttpViewRegistry` for views.

---

## `HttpChangeRegistry`

Collects command routes and registers them on an `http.ServeMux`.

```go
type HttpChangeRegistry struct { /* ... */ }

func (r *HttpChangeRegistry) RegisterCommand(
    pattern string,
    handler func(CommandRunner) http.HandlerFunc,
)

func (r HttpChangeRegistry) RegisterRoutes(mux *http.ServeMux, runner CommandRunner)

func (r HttpChangeRegistry) RegisteredRoutes() []string
```

### Pattern

`RegisterCommand` takes a Go 1.22+ HTTP pattern (`"METHOD /path/{param}"`) and a factory function. The factory receives a `CommandRunner` and returns an `http.HandlerFunc`. This allows handlers to close over the runner.

### Example

```go
var ChangeRegistry fairway.HttpChangeRegistry

func Register(registry *fairway.HttpChangeRegistry) {
    registry.RegisterCommand("POST /api/lists/{listId}", func(runner fairway.CommandRunner) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            var body struct {
                Name string `json:"name" validate:"required"`
            }
            if err := utils.JsonParse(r, &body); err != nil {
                http.Error(w, err.Error(), http.StatusBadRequest)
                return
            }

            if err := runner.RunPure(r.Context(), createListCommand{
                listId: r.PathValue("listId"),
                name:   body.Name,
            }); err != nil {
                http.Error(w, err.Error(), http.StatusInternalServerError)
                return
            }

            w.WriteHeader(http.StatusCreated)
        }
    })
}

func init() { Register(&ChangeRegistry) }
```

### Wiring in `main.go`

```go
ChangeRegistry.RegisterRoutes(mux, fairway.NewCommandRunner(store))
```

---

## `HttpViewRegistry`

Collects view routes and registers them on an `http.ServeMux`.

```go
type HttpViewRegistry struct { /* ... */ }

func (r *HttpViewRegistry) RegisterView(
    pattern string,
    handler func(EventsReader) http.HandlerFunc,
)

func (r HttpViewRegistry) RegisterRoutes(mux *http.ServeMux, client EventsReader)

func (r HttpViewRegistry) RegisteredRoutes() []string
```

### Example

```go
var ViewRegistry fairway.HttpViewRegistry

func Register(registry *fairway.HttpViewRegistry) {
    registry.RegisterView("GET /api/lists/{listId}", func(reader fairway.EventsReader) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // build projection and write response
        }
    })
}

func init() { Register(&ViewRegistry) }
```

### Wiring in `main.go`

```go
ViewRegistry.RegisterRoutes(mux, fairway.NewReader(store))
```

---

## Self-Registering Modules

The pattern throughout Fairway is to use `init()` for zero-coordination self-registration:

```go
// in change/createlist/createlist.go

var _ = func() struct{} {
    // or simply:
    return struct{}{}
}

func init() {
    change.ChangeRegistry.RegisterCommand("POST /api/lists/{listId}", handler)
}
```

`main.go` only needs to:

1. Create the store
2. Create the mux
3. Call `RegisterRoutes` on each registry
4. Start the server

All modules wire themselves automatically at import time. Adding a new module is a single `import` line in `main.go`.

---

## Complete `main.go` Example

```go
package main

import (
    "log"
    "net/http"

    "github.com/apple/foundationdb/bindings/go/src/fdb"
    "github.com/err0r500/fairway"
    "github.com/err0r500/fairway/dcb"

    _ "myapp/change/createlist"  // registers itself via init()
    _ "myapp/change/additem"
    _ "myapp/view/showlist"
    "myapp/change"
    "myapp/view"
)

func main() {
    fdb.MustAPIVersion(740)
    db := fdb.MustOpenDefault()
    store := dcb.NewDcbStore(db, "myapp")
    mux := http.NewServeMux()

    change.ChangeRegistry.RegisterRoutes(mux, fairway.NewCommandRunner(store))
    view.ViewRegistry.RegisterRoutes(mux, fairway.NewReader(store))

    log.Fatal(http.ListenAndServe(":8080", mux))
}
```
