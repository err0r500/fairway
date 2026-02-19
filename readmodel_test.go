package fairway_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestReadModelEventA struct {
	Value string
}

type TestReadModelEventB struct {
	Count int
}

func setupTestReadModel[T any](
	t *testing.T,
	dcbNs string,
	name string,
	examples []any,
	handler func(fairway.ScopedTx, fairway.Event) error,
	opts ...fairway.ReadModelOption[T],
) (*fairway.ReadModel[T], dcb.DcbStore) {
	t.Helper()

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	rm, err := fairway.NewReadModel[T](store, name, examples, handler, opts...)
	require.NoError(t, err)

	t.Cleanup(func() {
		rm.Stop()
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	return rm, store
}

func TestReadModel_BasicEventProcessing(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	var called atomic.Int32
	handler := func(_ fairway.ScopedTx, ev fairway.Event) error {
		called.Add(1)
		return nil
	}

	rm, store := setupTestReadModel[any](t, dcbNs, "test-rm",
		[]any{TestReadModelEventA{}},
		handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, rm.Start(ctx))

	dcbEvent, err := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "hello"}))
	require.NoError(t, err)
	require.NoError(t, store.Append(ctx, []dcb.Event{dcbEvent}, nil))

	assert.Eventually(t, func() bool {
		return called.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond, "handler should be called")
}

func TestReadModel_MultipleEventTypes(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	var calledA, calledB atomic.Int32
	handler := func(_ fairway.ScopedTx, ev fairway.Event) error {
		switch ev.Data.(type) {
		case TestReadModelEventA:
			calledA.Add(1)
		case TestReadModelEventB:
			calledB.Add(1)
		}
		return nil
	}

	rm, store := setupTestReadModel[any](t, dcbNs, "test-rm",
		[]any{TestReadModelEventA{}, TestReadModelEventB{}},
		handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, rm.Start(ctx))

	evA, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "x"}))
	evB, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventB{Count: 42}))
	require.NoError(t, store.Append(ctx, []dcb.Event{evA}, nil))
	require.NoError(t, store.Append(ctx, []dcb.Event{evB}, nil))

	assert.Eventually(t, func() bool {
		return calledA.Load() >= 1 && calledB.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond, "both event types should be handled")
}

func TestReadModel_CursorPersistence(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	ctx := context.Background()

	var called atomic.Int32
	handler := func(_ fairway.ScopedTx, ev fairway.Event) error {
		called.Add(1)
		return nil
	}

	// Append first event
	ev1, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "first"}))
	require.NoError(t, store.Append(ctx, []dcb.Event{ev1}, nil))

	// Start first read model instance, process first event, then stop
	rm1, err := fairway.NewReadModel[any](store, "my-projection", []any{TestReadModelEventA{}}, handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel1()
	require.NoError(t, rm1.Start(ctx1))

	assert.Eventually(t, func() bool {
		return called.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond)

	rm1.Stop()
	_ = rm1.Wait()

	// Append second event while stopped
	ev2, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "second"}))
	require.NoError(t, store.Append(ctx, []dcb.Event{ev2}, nil))

	countBefore := called.Load()

	// Start second instance — should resume from cursor, only process new event
	rm2, err := fairway.NewReadModel[any](store, "my-projection", []any{TestReadModelEventA{}}, handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx2, cancel2 := context.WithTimeout(ctx, 2*time.Second)
	defer cancel2()
	require.NoError(t, rm2.Start(ctx2))

	assert.Eventually(t, func() bool {
		return called.Load() > countBefore
	}, 2*time.Second, 10*time.Millisecond, "should process new event")

	rm2.Stop()

	assert.Equal(t, countBefore+1, called.Load(), "should only process events after cursor")
}

func TestReadModel_MultipleEvents(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	var called atomic.Int32
	handler := func(_ fairway.ScopedTx, ev fairway.Event) error {
		called.Add(1)
		return nil
	}

	rm, store := setupTestReadModel[any](t, dcbNs, "test-rm",
		[]any{TestReadModelEventA{}},
		handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, rm.Start(ctx))

	const n = 5
	for i := range n {
		ev, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: fmt.Sprintf("v%d", i)}))
		require.NoError(t, store.Append(ctx, []dcb.Event{ev}, nil))
	}

	assert.Eventually(t, func() bool {
		return called.Load() >= n
	}, 3*time.Second, 10*time.Millisecond, "all events should be processed")
}

func TestNewReadModel_ValidationErrors(t *testing.T) {
	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, "test-ns")
	handler := func(fairway.ScopedTx, fairway.Event) error { return nil }

	_, err := fairway.NewReadModel[any](nil, "name", []any{TestReadModelEventA{}}, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any](store, "", []any{TestReadModelEventA{}}, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any](store, "name", []any{TestReadModelEventA{}}, nil)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any](store, "name", nil, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any](store, "name", []any{}, handler)
	require.Error(t, err)
}

func TestReadModel_ScopedTxPrefixesKeys(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key("\x02" + dcbNs), End: fdb.Key("\x02" + dcbNs + "\xff")})
			return nil, nil
		})
	})

	var storedKey []byte
	handler := func(stx fairway.ScopedTx, ev fairway.Event) error {
		stx.Set(tuple.Tuple{"mykey", "sub"}, []byte("value"))
		return nil
	}

	rm, err := fairway.NewReadModel[any](store, "test-projection", []any{TestReadModelEventA{}}, handler,
		fairway.WithReadModelPollInterval[any](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, rm.Start(ctx))

	ev, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "test"}))
	require.NoError(t, store.Append(ctx, []dcb.Event{ev}, nil))

	// Wait for handler to process
	assert.Eventually(t, func() bool {
		var found bool
		_, _ = db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
			kvs := tr.GetRange(fdb.KeyRange{
				Begin: fdb.Key("\x02" + dcbNs),
				End:   fdb.Key("\x02" + dcbNs + "\xff"),
			}, fdb.RangeOptions{}).GetSliceOrPanic()

			for _, kv := range kvs {
				if containsSubspace(kv.Key, "rm", "test-projection", "data") {
					storedKey = kv.Key
					found = true
				}
			}
			return nil, nil
		})
		return found
	}, 2*time.Second, 20*time.Millisecond, "key should be stored with data prefix")

	rm.Stop()
	_ = rm.Wait()
	require.NotNil(t, storedKey, "key should be stored with data prefix")
}

func containsSubspace(key []byte, parts ...string) bool {
	keyStr := string(key)
	lastIdx := 0
	for _, part := range parts {
		idx := indexOf(keyStr[lastIdx:], part)
		if idx < 0 {
			return false
		}
		lastIdx += idx + len(part)
	}
	return true
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

type UserView struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestReadModel_Get(t *testing.T) {
	dcbNs := fmt.Sprintf("test-rm-%s", uuid.NewString())

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(dcbNs), End: fdb.Key(dcbNs + "\xff")})
			return nil, nil
		})
	})

	handler := func(stx fairway.ScopedTx, ev fairway.Event) error {
		e := ev.Data.(TestReadModelEventA)
		user := UserView{Name: e.Value, Email: e.Value + "@example.com"}
		data, _ := json.Marshal(user)
		stx.Set(tuple.Tuple{"user", e.Value}, data)
		return nil
	}

	rm, err := fairway.NewReadModel[UserView](store, "user-view", []any{TestReadModelEventA{}}, handler,
		fairway.WithReadModelPollInterval[UserView](10*time.Millisecond),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, rm.Start(ctx))
	defer rm.Stop()

	ev, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "alice"}))
	require.NoError(t, store.Append(ctx, []dcb.Event{ev}, nil))

	var results []*UserView
	assert.Eventually(t, func() bool {
		var err error
		results, err = rm.Get(tuple.Tuple{"user", "alice"})
		return err == nil && len(results) == 1 && results[0] != nil
	}, 2*time.Second, 10*time.Millisecond, "should get user alice")

	require.NotNil(t, results[0])
	assert.Equal(t, "alice", results[0].Name)
	assert.Equal(t, "alice@example.com", results[0].Email)

	// Missing key returns nil
	results, err = rm.Get(tuple.Tuple{"user", "bob"})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0])
}
