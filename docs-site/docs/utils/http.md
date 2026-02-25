# HTTP Utilities

The `utils/` package provides reusable HTTP helpers for use in command and view handlers.

---

## `JsonParse`

Decodes a JSON request body and validates the result using struct tags.

```go
func JsonParse[T any](r *http.Request, v *T) error
```

- Decodes `r.Body` into `v` using `encoding/json`
- Validates `v` using `go-playground/validator` struct tags
- Returns the first error encountered (decode or validation)

### Example

```go
var body struct {
    Name  string `json:"name"  validate:"required"`
    Email string `json:"email" validate:"required,email"`
}

if err := utils.JsonParse(r, &body); err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}
```

Supported validation tags come from `github.com/go-playground/validator/v10`. Common tags:

| Tag | Meaning |
|---|---|
| `required` | Field must be non-zero |
| `email` | Must be a valid email address |
| `min=N` | Minimum length / value |
| `max=N` | Maximum length / value |
| `oneof=a b` | Must be one of the listed values |

---

## `IdempotencyMiddleware`

An HTTP middleware that deduplicates requests sharing the same `Idempotency-Key` header, backed by FoundationDB.

```go
func IdempotencyMiddleware(db fdb.Database, namespace string, next http.Handler) http.Handler
```

### Behaviour

1. If no `Idempotency-Key` header is present, the request passes through unchanged.
2. If the key is **new**: the middleware marks it as "processing" in FDB, runs the handler, stores the response (status code + body), and returns it.
3. If the key is **already processing**: the middleware polls FDB (every 50ms, up to 10s) until the result is available, then returns it.
4. If the key is **already complete**: the stored response is returned immediately without running the handler again.

### Storage

Responses are stored in `<namespace>/idempotency/<key>` as a binary-encoded blob:

```
4 bytes (big-endian uint32): HTTP status code
N bytes: response body
```

### Example

```go
mux := http.NewServeMux()
// ... register routes ...

handler := utils.IdempotencyMiddleware(db, "myapp", mux)
log.Fatal(http.ListenAndServe(":8080", handler))
```

Clients use it by including a unique key per logical operation:

```bash
curl -X POST http://localhost:8080/api/lists/my-list \
     -H "Idempotency-Key: create-list-550e8400" \
     -H "Content-Type: application/json" \
     -d '{"name": "Shopping"}'
```

Retrying the same request with the same key returns the original response without re-executing the command.

### Timeout

If the first request does not complete within 10 seconds, waiting duplicates receive `504 Gateway Timeout`.

---

## Constants

```go
const (
    idempotencyHeader         = "Idempotency-Key"
    idempotencyDefaultTimeout = 10 * time.Second
    idempotencyPollInterval   = 50 * time.Millisecond
)
```
