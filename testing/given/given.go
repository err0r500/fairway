package given

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/err0r500/fairway"
	"github.com/err0r500/fairway/dcb"
	"resty.dev/v3"
)

// EventsInStore appends events to store without condition (for test setup)
func EventsInStore(store dcb.DcbStore, e fairway.TaggedEvent, ee ...fairway.TaggedEvent) {
	ctx := context.Background()

	allEvents := append([]fairway.TaggedEvent{e}, ee...)
	dcbEvents := make([]dcb.Event, len(allEvents))

	for i, te := range allEvents {
		dcbEvent, err := fairway.ToDcbEvent(te)
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
	store := dcb.SetupTestStore(t)
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
