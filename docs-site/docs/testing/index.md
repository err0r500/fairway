# Testing Helpers

The `testing/` package provides a structured set of test utilities following the **given / when / then** pattern.

```
testing/
├── given/   — Set up test state (store, server, fixtures)
├── when/    — Perform actions (HTTP requests)
└── then/    — Assert outcomes (events in store)
```

---

## Philosophy

Test helpers in Fairway mirror how you think about behaviour:

```
GIVEN   some events already exist in the store
WHEN    an HTTP request is made
THEN    specific events are present in the store
```

Each package handles exactly one of these concerns.

---

## Quick Example

```go
func TestCreateList(t *testing.T) {
    // GIVEN: a fresh store and HTTP server wired to our command handler
    store, server, client := given.FreshSetup(t, Register)

    // WHEN: we POST to create a list
    resp, err := client.R().
        SetBody(map[string]string{"name": "Shopping"}).
        Post(server.URL + "/api/lists/my-list")

    // THEN: the response is 201 and the event is in the store
    assert.NoError(t, err)
    assert.Equal(t, http.StatusCreated, resp.StatusCode())

    then.ExpectEventsInStore(t, store,
        fairway.NewEvent(ListCreated{ListId: "my-list", Name: "Shopping"}),
    )
}
```

---

## Pages

- [given — Setup](given.md) — `FreshSetup`, `SetupTestStore`, `EventsInStore`
- [when — Actions](when.md) — `HttpPostJSON`
- [then — Assertions](then.md) — `ExpectEventsInStore`
