package given

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"github.com/google/uuid"
	"resty.dev/v3"
)

// EventsInStore appends events to store without condition (for test setup)
func EventsInStore(store dcb.DcbStore, e fairway.Event, ee ...fairway.Event) {
	ctx := context.Background()

	allEvents := append([]fairway.Event{e}, ee...)
	dcbEvents := make([]dcb.Event, len(allEvents))

	for i, ev := range allEvents {
		dcbEvent, err := fairway.ToDcbEvent(ev)
		if err != nil {
			panic(err)
		}
		dcbEvents[i] = dcbEvent
	}

	if err := store.Append(ctx, dcbEvents, nil); err != nil {
		panic(err)
	}
}

func FreshSetup(t *testing.T, registerFn any) (dcb.DcbStore, *httptest.Server, *resty.Client) {
	store := SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)
	mux := http.NewServeMux()

	// Use reflection to determine registry type
	fnType := reflect.TypeOf(registerFn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		panic("registerFn must be a function")
	}
	if fnType.NumIn() != 1 {
		panic("registerFn must accept exactly 1 parameter")
	}

	paramType := fnType.In(0)
	if paramType.Kind() != reflect.Ptr {
		panic("registerFn parameter must be a pointer")
	}

	elemType := paramType.Elem()
	fnValue := reflect.ValueOf(registerFn)

	switch elemType.Name() {
	case "HttpChangeRegistry":
		changeRegistry := &fairway.HttpChangeRegistry{}
		fnValue.Call([]reflect.Value{reflect.ValueOf(changeRegistry)})
		changeRegistry.RegisterRoutes(mux, runner)
	case "HttpViewRegistry":
		viewRegistry := &fairway.HttpViewRegistry{}
		fnValue.Call([]reflect.Value{reflect.ValueOf(viewRegistry)})
		reader := fairway.NewReader(store)
		viewRegistry.RegisterRoutes(mux, reader)
	default:
		panic("registerFn must accept *fairway.HttpChangeRegistry or *fairway.HttpViewRegistry")
	}

	server := httptest.NewServer(mux)
	httpClient := resty.New()
	t.Cleanup(func() {
		server.Close()
		httpClient.Close()
	})
	return store, server, httpClient
}

// FreshSetupWithIdempotency is like FreshSetup but configures the change registry
// with an FDB-backed idempotency store (24h TTL).
func FreshSetupWithIdempotency(t *testing.T, registerFn any) (dcb.DcbStore, *httptest.Server, *resty.Client) {
	store := SetupTestStore(t)
	runner := fairway.NewCommandRunner(store)
	mux := http.NewServeMux()

	idempotencyStore := dcb.NewIdempotencyStore(store.Database(), store.Namespace(), 24*time.Hour)

	fnType := reflect.TypeOf(registerFn)
	if fnType == nil || fnType.Kind() != reflect.Func {
		panic("registerFn must be a function")
	}
	if fnType.NumIn() != 1 {
		panic("registerFn must accept exactly 1 parameter")
	}

	paramType := fnType.In(0)
	if paramType.Kind() != reflect.Ptr {
		panic("registerFn parameter must be a pointer")
	}

	elemType := paramType.Elem()
	fnValue := reflect.ValueOf(registerFn)

	switch elemType.Name() {
	case "HttpChangeRegistry":
		changeRegistry := &fairway.HttpChangeRegistry{}
		fnValue.Call([]reflect.Value{reflect.ValueOf(changeRegistry)})
		changeRegistry.WithIdempotency(idempotencyStore)
		changeRegistry.RegisterRoutes(mux, runner)
	default:
		panic("FreshSetupWithIdempotency only supports *fairway.HttpChangeRegistry")
	}

	server := httptest.NewServer(mux)
	httpClient := resty.New()
	t.Cleanup(func() {
		server.Close()
		httpClient.Close()
	})
	return store, server, httpClient
}

func SetupTestStore(t *testing.T) dcb.DcbStore {
	t.Helper()

	fdb.MustAPIVersion(740)
	db := fdb.MustOpenDefault()

	// Use unique namespace per test
	namespace := fmt.Sprintf("t-%d", uuid.New())
	store := dcb.NewDcbStore(db, namespace)

	// Clean up after test
	t.Cleanup(func() {
		_, _ = db.Transact(func(tr fdb.Transaction) (any, error) {
			tr.ClearRange(fdb.KeyRange{Begin: fdb.Key(namespace), End: fdb.Key(namespace + "\xff")})
			return nil, nil
		})
	})

	return store
}
