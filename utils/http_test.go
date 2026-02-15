package utils_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyMiddleware_ConcurrentSameKey(t *testing.T) {
	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()
	namespace := fmt.Sprintf("idem-test-%s", uuid.New().String())

	// Clean up FDB keys after test.
	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{
				Begin: fdb.Key(namespace),
				End:   fdb.Key(namespace + "\xff"),
			})
			return nil, nil
		})
	})

	var handlerCalls atomic.Int32

	// The "real" handler increments a counter and returns a fixed response.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalls.Add(1)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	handler := utils.IdempotencyMiddleware(db, namespace, inner)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	const concurrency = 5
	idempotencyKey := uuid.New().String()

	var wg sync.WaitGroup
	statusCodes := make([]int, concurrency)
	bodies := make([]string, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost, server.URL+"/test", nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Idempotency-Key", idempotencyKey)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()

			var buf [1024]byte
			n, _ := resp.Body.Read(buf[:])
			statusCodes[idx] = resp.StatusCode
			bodies[idx] = string(buf[:n])
		}(i)
	}

	wg.Wait()

	// Only ONE call to the inner handler.
	assert.Equal(t, int32(1), handlerCalls.Load(), "inner handler must be called exactly once")

	// All 5 responses must be identical: 201 + {"ok":true}.
	for i := range concurrency {
		assert.Equal(t, http.StatusCreated, statusCodes[i], "request %d status", i)
		assert.Equal(t, `{"ok":true}`, bodies[i], "request %d body", i)
	}
}
