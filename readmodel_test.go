package fairway_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/utils"
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

// TestRepo wraps utils.KV for test handlers
type TestRepo struct {
	kv     utils.KV
	called *atomic.Int32
}

func (r TestRepo) RecordCall() {
	if r.called != nil {
		r.called.Add(1)
	}
}

func (r TestRepo) SetJSON(key fairway.Path, v any) error {
	return r.kv.SetJSON(key, v)
}

func (r TestRepo) SetPath(key []string) {
	r.kv.SetPath(key)
}

func setupTestReadModel[T any](
	t *testing.T,
	dcbNs string,
	name string,
	examples []any,
	called *atomic.Int32,
	handler func(TestRepo, fairway.Event) error,
	opts ...fairway.ReadModelOption[T, TestRepo],
) (*fairway.ReadModel[T, TestRepo], dcb.DcbStore) {
	t.Helper()

	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	repoFactory := func(tr fdb.Transaction, space subspace.Subspace) TestRepo {
		return TestRepo{kv: utils.NewKV(tr, space), called: called}
	}

	rm, err := fairway.NewReadModel[T, TestRepo](store, name, examples, repoFactory, handler, opts...)
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
	handler := func(repo TestRepo, ev fairway.Event) error {
		repo.RecordCall()
		return nil
	}

	rm, store := setupTestReadModel[any](t, dcbNs, "test-rm",
		[]any{TestReadModelEventA{}},
		&called,
		handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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
	handler := func(repo TestRepo, ev fairway.Event) error {
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
		nil,
		handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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
	handler := func(repo TestRepo, ev fairway.Event) error {
		repo.RecordCall()
		return nil
	}
	repoFactory := func(tr fdb.Transaction, space subspace.Subspace) TestRepo {
		return TestRepo{kv: utils.NewKV(tr, space), called: &called}
	}

	// Append first event
	ev1, _ := fairway.ToDcbEvent(fairway.NewEvent(TestReadModelEventA{Value: "first"}))
	require.NoError(t, store.Append(ctx, []dcb.Event{ev1}, nil))

	// Start first read model instance, process first event, then stop
	rm1, err := fairway.NewReadModel[any, TestRepo](store, "my-projection", []any{TestReadModelEventA{}}, repoFactory, handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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
	rm2, err := fairway.NewReadModel[any, TestRepo](store, "my-projection", []any{TestReadModelEventA{}}, repoFactory, handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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
	handler := func(repo TestRepo, ev fairway.Event) error {
		repo.RecordCall()
		return nil
	}

	rm, store := setupTestReadModel[any](t, dcbNs, "test-rm",
		[]any{TestReadModelEventA{}},
		&called,
		handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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
	handler := func(TestRepo, fairway.Event) error { return nil }
	repoFactory := func(tr fdb.Transaction, space subspace.Subspace) TestRepo {
		return TestRepo{}
	}

	_, err := fairway.NewReadModel[any, TestRepo](nil, "name", []any{TestReadModelEventA{}}, repoFactory, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any, TestRepo](store, "", []any{TestReadModelEventA{}}, repoFactory, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any, TestRepo](store, "name", []any{TestReadModelEventA{}}, repoFactory, nil)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any, TestRepo](store, "name", nil, repoFactory, handler)
	require.Error(t, err)

	_, err = fairway.NewReadModel[any, TestRepo](store, "name", []any{}, repoFactory, handler)
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
	handler := func(repo TestRepo, ev fairway.Event) error {
		repo.SetPath([]string{"mykey", "sub"})
		return nil
	}
	repoFactory := func(tr fdb.Transaction, space subspace.Subspace) TestRepo {
		return TestRepo{kv: utils.NewKV(tr, space)}
	}

	rm, err := fairway.NewReadModel[any, TestRepo](store, "test-projection", []any{TestReadModelEventA{}}, repoFactory, handler,
		fairway.WithReadModelPollInterval[any, TestRepo](10*time.Millisecond),
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

type UserRepo struct {
	kv utils.KV
}

func (r UserRepo) SetJSON(key fairway.Path, v any) error {
	return r.kv.SetJSON(key, v)
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

	handler := func(repo UserRepo, ev fairway.Event) error {
		e := ev.Data.(TestReadModelEventA)
		user := UserView{Name: e.Value, Email: e.Value + "@example.com"}
		_ = repo.SetJSON(fairway.P("user", e.Value), user)
		return nil
	}
	repoFactory := func(tr fdb.Transaction, space subspace.Subspace) UserRepo {
		return UserRepo{kv: utils.NewKV(tr, space)}
	}

	rm, err := fairway.NewReadModel[UserView, UserRepo](store, "user-view", []any{TestReadModelEventA{}}, repoFactory, handler,
		fairway.WithReadModelPollInterval[UserView, UserRepo](10*time.Millisecond),
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
		results, err = rm.Get(ctx, fairway.P("user", "alice"))
		return err == nil && len(results) == 1 && results[0] != nil
	}, 2*time.Second, 10*time.Millisecond, "should get user alice")

	require.NotNil(t, results[0])
	assert.Equal(t, "alice", results[0].Name)
	assert.Equal(t, "alice@example.com", results[0].Email)

	// Missing key returns nil
	results, err = rm.Get(ctx, fairway.P("user", "bob"))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Nil(t, results[0])
}
