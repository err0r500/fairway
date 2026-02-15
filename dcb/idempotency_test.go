package dcb_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway/dcb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyStore_CheckMissing(t *testing.T) {
	t.Parallel()
	store := setupIdempotencyStore(t, 24*time.Hour)

	_, found, err := store.Check(context.Background(), "nonexistent-key")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestIdempotencyStore_StoreAndCheck(t *testing.T) {
	t.Parallel()
	store := setupIdempotencyStore(t, 24*time.Hour)
	ctx := context.Background()

	err := store.Store(ctx, "key-1", 201)
	require.NoError(t, err)

	statusCode, found, err := store.Check(ctx, "key-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 201, statusCode)
}

func TestIdempotencyStore_DifferentKeys(t *testing.T) {
	t.Parallel()
	store := setupIdempotencyStore(t, 24*time.Hour)
	ctx := context.Background()

	require.NoError(t, store.Store(ctx, "key-a", 201))
	require.NoError(t, store.Store(ctx, "key-b", 409))

	statusA, foundA, err := store.Check(ctx, "key-a")
	require.NoError(t, err)
	assert.True(t, foundA)
	assert.Equal(t, 201, statusA)

	statusB, foundB, err := store.Check(ctx, "key-b")
	require.NoError(t, err)
	assert.True(t, foundB)
	assert.Equal(t, 409, statusB)
}

func TestIdempotencyStore_OverwriteKey(t *testing.T) {
	t.Parallel()
	store := setupIdempotencyStore(t, 24*time.Hour)
	ctx := context.Background()

	require.NoError(t, store.Store(ctx, "key-1", 201))
	require.NoError(t, store.Store(ctx, "key-1", 500))

	statusCode, found, err := store.Check(ctx, "key-1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 500, statusCode)
}

func TestIdempotencyStore_TTLExpired(t *testing.T) {
	t.Parallel()
	// TTL of 0 means all keys are immediately expired
	store := setupIdempotencyStore(t, 0)
	ctx := context.Background()

	require.NoError(t, store.Store(ctx, "key-1", 201))

	_, found, err := store.Check(ctx, "key-1")
	require.NoError(t, err)
	assert.False(t, found, "key should be expired")
}

func TestIdempotencyStore_CancelledContext(t *testing.T) {
	t.Parallel()
	store := setupIdempotencyStore(t, 24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := store.Check(ctx, "key-1")
	assert.ErrorIs(t, err, context.Canceled)

	err = store.Store(ctx, "key-1", 201)
	assert.ErrorIs(t, err, context.Canceled)
}

func setupIdempotencyStore(t *testing.T, ttl time.Duration) *dcb.FdbIdempotencyStore {
	t.Helper()

	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()
	namespace := fmt.Sprintf("t-%d", uuid.New())

	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(namespace), End: fdb.Key(namespace + "\xff")})
			return nil, nil
		})
	})

	return dcb.NewIdempotencyStore(db, namespace, ttl)
}
