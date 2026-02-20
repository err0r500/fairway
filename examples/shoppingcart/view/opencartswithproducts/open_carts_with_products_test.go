package opencartswithproducts_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/err0r500/fairway/examples/shoppingcart/event"
	"github.com/err0r500/fairway/examples/shoppingcart/view/opencartswithproducts"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	fdb.MustAPIVersion(740)
	os.Exit(m.Run())
}

func setupTestRM(t *testing.T) (*fairway.ReadModel[opencartswithproducts.CartItem, opencartswithproducts.Repo], dcb.DcbStore) {
	t.Helper()
	dcbNs := fmt.Sprintf("test-opencarts-%s", uuid.NewString())
	db := fdb.MustOpenDefault()
	store := dcb.NewDcbStore(db, dcbNs)

	rm, err := opencartswithproducts.NewReadModel(store)
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

func appendEvent(t *testing.T, ctx context.Context, store dcb.DcbStore, data any) {
	t.Helper()
	ev, err := fairway.ToDcbEvent(fairway.NewEvent(data))
	require.NoError(t, err)
	require.NoError(t, store.Append(ctx, []dcb.Event{ev}, nil))
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	require.Eventually(t, condition, 2*time.Second, 10*time.Millisecond)
}

func TestOpenCartsWithProducts_AddItem(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{
		CartId: "cart-1", ItemId: "item-1", ProductId: "product-A",
	})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 1
	})

	keys, err := rm.Scan(ctx, fairway.P("product-A"))
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "cart-1", keys[0][1])
	assert.Equal(t, "item-1", keys[0][2])
}

func TestOpenCartsWithProducts_MultipleItemsSameProduct(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-1", ProductId: "product-A"})
	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-2", ItemId: "item-2", ProductId: "product-A"})
	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-3", ProductId: "product-B"})

	waitFor(t, func() bool {
		keysA, _ := rm.Scan(ctx, fairway.P("product-A"))
		keysB, _ := rm.Scan(ctx, fairway.P("product-B"))
		return len(keysA) == 2 && len(keysB) == 1
	})

	keys, err := rm.Scan(ctx, fairway.P("product-A"))
	require.NoError(t, err)
	assert.Len(t, keys, 2)

	keys, err = rm.Scan(ctx, fairway.P("product-B"))
	require.NoError(t, err)
	assert.Len(t, keys, 1)
}

func TestOpenCartsWithProducts_RemoveItem(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-1", ProductId: "product-A"})
	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-2", ProductId: "product-A"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 2
	})

	appendEvent(t, ctx, store, event.ItemRemoved{CartId: "cart-1", ItemId: "item-1"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 1
	})

	keys, err := rm.Scan(ctx, fairway.P("product-A"))
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "item-2", keys[0][2])
}

func TestOpenCartsWithProducts_ClearCart(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-1", ProductId: "product-A"})
	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-2", ProductId: "product-B"})
	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-2", ItemId: "item-3", ProductId: "product-A"})

	waitFor(t, func() bool {
		keysA, _ := rm.Scan(ctx, fairway.P("product-A"))
		keysB, _ := rm.Scan(ctx, fairway.P("product-B"))
		return len(keysA) == 2 && len(keysB) == 1
	})

	appendEvent(t, ctx, store, event.CartCleared{CartId: "cart-1"})

	waitFor(t, func() bool {
		keysA, _ := rm.Scan(ctx, fairway.P("product-A"))
		keysB, _ := rm.Scan(ctx, fairway.P("product-B"))
		return len(keysA) == 1 && len(keysB) == 0
	})

	keysA, _ := rm.Scan(ctx, fairway.P("product-A"))
	assert.Len(t, keysA, 1)
	assert.Equal(t, "cart-2", keysA[0][1])

	keysB, _ := rm.Scan(ctx, fairway.P("product-B"))
	assert.Len(t, keysB, 0)
}

func TestOpenCartsWithProducts_SubmitCart(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-1", ProductId: "product-A"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 1
	})

	appendEvent(t, ctx, store, event.CartSubmitted{CartId: "cart-1"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 0
	})

	keys, _ := rm.Scan(ctx, fairway.P("product-A"))
	assert.Len(t, keys, 0)
}

func TestOpenCartsWithProducts_ItemArchived(t *testing.T) {
	rm, store := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	appendEvent(t, ctx, store, event.ItemAdded{CartId: "cart-1", ItemId: "item-1", ProductId: "product-A"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 1
	})

	appendEvent(t, ctx, store, event.ItemArchived{CartId: "cart-1", ItemId: "item-1"})

	waitFor(t, func() bool {
		keys, _ := rm.Scan(ctx, fairway.P("product-A"))
		return len(keys) == 0
	})

	keys, _ := rm.Scan(ctx, fairway.P("product-A"))
	assert.Len(t, keys, 0)
}

func TestOpenCartsWithProducts_EmptyResult(t *testing.T) {
	rm, _ := setupTestRM(t)
	ctx := context.Background()
	require.NoError(t, rm.Start(ctx))

	waitFor(t, rm.IsCaughtUp)

	keys, err := rm.Scan(ctx, fairway.P("nonexistent"))
	require.NoError(t, err)
	assert.Len(t, keys, 0)
}
