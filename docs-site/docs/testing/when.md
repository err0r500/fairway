# when — Actions

The `testing/when` package provides helpers for performing actions in tests — primarily HTTP requests.

```go
import "github.com/err0r500/fairway/testing/when"
```

---

## `HttpPostJSON`

Makes an HTTP POST request with a JSON-encoded body.

```go
func HttpPostJSON[T json.Marshaler](url string, t *T) (*http.Response, error)
```

- Marshals `*t` to JSON
- Posts to `url` with `Content-Type: application/json`
- Returns the raw `*http.Response`

### Example

```go
type CreateListRequest struct {
    Name string `json:"name"`
}

func (r CreateListRequest) MarshalJSON() ([]byte, error) {
    return json.Marshal(struct{ Name string }{r.Name})
}

resp, err := when.HttpPostJSON(
    server.URL+"/api/lists/my-list",
    &CreateListRequest{Name: "Shopping"},
)
assert.NoError(t, err)
assert.Equal(t, http.StatusCreated, resp.StatusCode)
```

!!! note
    In most tests, you will use the `*resty.Client` returned by `given.FreshSetup` directly, which provides a more ergonomic API. `HttpPostJSON` is a lower-level helper for cases where you need the raw `*http.Response`.

### Resty Alternative

The `resty.Client` returned by `given.FreshSetup` is generally preferred:

```go
_, server, client := given.FreshSetup(t, Register)

resp, err := client.R().
    SetBody(map[string]string{"name": "Shopping"}).
    Post(server.URL + "/api/lists/my-list")

assert.NoError(t, err)
assert.Equal(t, 201, resp.StatusCode())
```
