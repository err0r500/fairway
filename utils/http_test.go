package utils_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/err0r500/fairway/testing/given"
	"github.com/err0r500/fairway/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIdempotencyMiddleware_ConcurrentSameKey(t *testing.T) {
	// given
	store := given.SetupTestStore(t)
	var handlerCalls atomic.Int32

	server := httptest.NewServer(
		utils.IdempotencyMiddleware(
			store.Database(),
			store.Namespace(),
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalls.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"ok":true}`))
			}),
		))
	t.Cleanup(server.Close)

	const concurrentReqCount = 5
	statusCodes := make([]int, concurrentReqCount)
	bodies := make([]string, concurrentReqCount)

	// when
	{
		idempotencyKey := uuid.New().String()

		var wg sync.WaitGroup

		for i := range concurrentReqCount {
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
	}

	// then
	assert.Equal(t, int32(1), handlerCalls.Load(), "inner handler must be called exactly once")
	for i := range concurrentReqCount {
		assert.Equal(t, http.StatusCreated, statusCodes[i], "request %d status", i)
		assert.Equal(t, `{"ok":true}`, bodies[i], "request %d body", i)
	}
}
