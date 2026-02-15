package dcb

import (
	"context"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
)

// IdempotencyStore checks and records idempotency keys to prevent duplicate processing.
// Keys are stored with a TTL; expired keys are treated as absent.
type IdempotencyStore interface {
	// Check returns the cached status code for a key, or found=false if the key
	// does not exist or has expired.
	Check(ctx context.Context, key string) (statusCode int, found bool, err error)

	// Store records a key with its associated status code. The key will expire
	// after the TTL configured at construction time.
	Store(ctx context.Context, key string, statusCode int) error
}

// FdbIdempotencyStore implements IdempotencyStore using FoundationDB.
// Storage layout: /<namespace>/i/<key> → packed(status_code, created_at_unix_nano)
type FdbIdempotencyStore struct {
	db  fdb.Database
	ss  subspace.Subspace
	ttl time.Duration
}

// NewIdempotencyStore creates an FDB-backed idempotency store.
// Keys expire after ttl duration.
func NewIdempotencyStore(db fdb.Database, namespace string, ttl time.Duration) *FdbIdempotencyStore {
	root := subspace.Sub(namespace)
	return &FdbIdempotencyStore{
		db:  db,
		ss:  root.Sub("i"),
		ttl: ttl,
	}
}

func (s *FdbIdempotencyStore) Check(ctx context.Context, key string) (int, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}

	result, err := s.db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
		val := tr.Get(s.ss.Pack(tuple.Tuple{key})).MustGet()
		if val == nil {
			return nil, nil
		}

		t, err := tuple.Unpack(val)
		if err != nil {
			return nil, err
		}
		if len(t) < 2 {
			return nil, nil
		}

		statusCode, ok1 := t[0].(int64)
		createdNano, ok2 := t[1].(int64)
		if !ok1 || !ok2 {
			return nil, nil
		}

		createdAt := time.Unix(0, createdNano)
		if time.Since(createdAt) > s.ttl {
			return nil, nil // expired
		}

		return int(statusCode), nil
	})
	if err != nil {
		return 0, false, err
	}

	if result == nil {
		return 0, false, nil
	}

	return result.(int), true, nil
}

func (s *FdbIdempotencyStore) Store(ctx context.Context, key string, statusCode int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	_, err := s.db.Transact(func(tr fdb.Transaction) (any, error) {
		val := tuple.Tuple{int64(statusCode), time.Now().UnixNano()}.Pack()
		tr.Set(s.ss.Pack(tuple.Tuple{key}), val)
		return nil, nil
	})
	return err
}
