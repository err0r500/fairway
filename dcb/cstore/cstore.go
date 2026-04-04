// Package cstore provides a DcbStore implementation backed by the Rust
// fairway_fdb_c shared library via cgo.
//
// This allows Go to use the same DCB store implementation as the Elixir port,
// with FoundationDB accessed through a single canonical Rust codebase.
//
// # Build prerequisites
//
// Compile the Rust library first:
//
//	cd fairway_ex/native && cargo build --release -p fairway_fdb_c
//
// Then set CGO_LDFLAGS and CGO_CFLAGS, or install the library system-wide:
//
//	export CGO_LDFLAGS="-L$(pwd)/fairway_ex/native/target/release"
//	export CGO_CFLAGS="-I$(pwd)/fairway_ex/native/fairway_fdb_c/include"
//	export LD_LIBRARY_PATH="$(pwd)/fairway_ex/native/target/release:$LD_LIBRARY_PATH"
//
// # Usage
//
//	store, err := cstore.Open("/etc/foundationdb/fdb.cluster", "my_app")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
//
//	runner := fairway.NewCommandRunner(store)

// #cgo LDFLAGS: -lfairway_fdb_c
// #cgo CFLAGS: -I${SRCDIR}/../../fairway_ex/native/fairway_fdb_c/include
// #include "fairway_dcb.h"
// #include <stdlib.h>
import "C"

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"unsafe"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/err0r500/fairway/dcb"
)

// Store is a DcbStore backed by the Rust fairway_fdb_c library.
type Store struct {
	handle    C.FwDb
	namespace string
	db        fdb.Database // opened separately for DcbStore.Database() compat
}

// Open opens an FDB database via the Rust library.
// clusterFile may be "" to use the default cluster file.
// namespace scopes all keys (e.g. "my_app_production").
func Open(clusterFile, namespace string) (*Store, error) {
	var cCluster *C.char
	if clusterFile != "" {
		cCluster = C.CString(clusterFile)
		defer C.free(unsafe.Pointer(cCluster))
	}

	cNamespace := C.CString(namespace)
	defer C.free(unsafe.Pointer(cNamespace))

	handle := C.fw_open_db(cCluster, cNamespace)
	if handle == nil {
		return nil, errors.New("fairway_fdb_c: fw_open_db returned NULL")
	}

	// Also open via Go bindings for Database() compatibility
	fdb.MustAPIVersion(710)
	var goDb fdb.Database
	var err error
	if clusterFile != "" {
		goDb, err = fdb.OpenDatabase(clusterFile)
	} else {
		goDb, err = fdb.OpenDefault()
	}
	if err != nil {
		C.fw_close_db(handle)
		return nil, fmt.Errorf("fdb.OpenDefault: %w", err)
	}

	return &Store{handle: handle, namespace: namespace, db: goDb}, nil
}

// Close releases the database handle.
func (s *Store) Close() {
	if s.handle != nil {
		C.fw_close_db(s.handle)
		s.handle = nil
	}
}

// ── DcbStore interface ────────────────────────────────────────────────────────

func (s *Store) Database() fdb.Database { return s.db }
func (s *Store) Namespace() string      { return s.namespace }

// Append atomically appends events with optional DCB conditions.
func (s *Store) Append(_ context.Context, events []dcb.Event, conditions ...dcb.AppendCondition) error {
	eventsJSON, err := marshalEvents(events)
	if err != nil {
		return fmt.Errorf("cstore: marshal events: %w", err)
	}
	condsJSON, err := marshalConditions(conditions)
	if err != nil {
		return fmt.Errorf("cstore: marshal conditions: %w", err)
	}

	cEvents := C.CString(eventsJSON)
	defer C.free(unsafe.Pointer(cEvents))
	cConds := C.CString(condsJSON)
	defer C.free(unsafe.Pointer(cConds))

	raw := C.fw_append_events(s.handle, cEvents, cConds)
	defer C.fw_free_string(raw)

	return parseAppendResult(C.GoString(raw))
}

// Read returns events matching the query as an iterator.
func (s *Store) Read(_ context.Context, query dcb.Query, opts *dcb.ReadOptions) iter.Seq2[dcb.StoredEvent, error] {
	return func(yield func(dcb.StoredEvent, error) bool) {
		queryJSON, err := marshalQuery(query)
		if err != nil {
			yield(dcb.StoredEvent{}, fmt.Errorf("cstore: marshal query: %w", err))
			return
		}
		optsJSON, err := marshalReadOpts(opts)
		if err != nil {
			yield(dcb.StoredEvent{}, fmt.Errorf("cstore: marshal opts: %w", err))
			return
		}

		cQuery := C.CString(queryJSON)
		defer C.free(unsafe.Pointer(cQuery))
		cOpts := C.CString(optsJSON)
		defer C.free(unsafe.Pointer(cOpts))

		raw := C.fw_read_events(s.handle, cQuery, cOpts)
		defer C.fw_free_string(raw)

		events, err := parseEventsResult(C.GoString(raw))
		if err != nil {
			yield(dcb.StoredEvent{}, err)
			return
		}
		for _, e := range events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// ReadAll returns all events in the namespace in versionstamp order.
func (s *Store) ReadAll(_ context.Context) iter.Seq2[dcb.StoredEvent, error] {
	return func(yield func(dcb.StoredEvent, error) bool) {
		raw := C.fw_read_all_events(s.handle)
		defer C.fw_free_string(raw)

		events, err := parseEventsResult(C.GoString(raw))
		if err != nil {
			yield(dcb.StoredEvent{}, err)
			return
		}
		for _, e := range events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// ── JSON wire types (must match fairway_fdb_core serde structs) ───────────────

type wireEvent struct {
	Type    string   `json:"type"`
	Tags    []string `json:"tags"`
	DataB64 string   `json:"data_b64"`
}

type wireQueryItem struct {
	Types []string `json:"types"`
	Tags  []string `json:"tags"`
}

type wireCondition struct {
	QueryItems    []wireQueryItem `json:"query_items"`
	AfterPosition *string         `json:"after_position"` // hex24 or null
}

type wireReadOpts struct {
	Limit         *int    `json:"limit,omitempty"`
	AfterPosition *string `json:"after_position,omitempty"` // hex24
	Reverse       bool    `json:"reverse"`
}

type wireReadResponse struct {
	Events []wireStoredEvent `json:"events"`
	Error  *string           `json:"error"`
}

type wireStoredEvent struct {
	Position  string   `json:"position"` // hex24
	EventType string   `json:"type"`
	Tags      []string `json:"tags"`
	DataB64   string   `json:"data_b64"`
}

type wireAppendResponse struct {
	Ok    *bool   `json:"ok"`
	Error *string `json:"error"`
}

// ── Marshalling ───────────────────────────────────────────────────────────────

func marshalEvents(events []dcb.Event) (string, error) {
	wires := make([]wireEvent, len(events))
	for i, e := range events {
		wires[i] = wireEvent{
			Type:    e.Type,
			Tags:    e.Tags,
			DataB64: base64.StdEncoding.EncodeToString(e.Data),
		}
	}
	b, err := json.Marshal(wires)
	return string(b), err
}

func marshalConditions(conditions []dcb.AppendCondition) (string, error) {
	wires := make([]wireCondition, len(conditions))
	for i, c := range conditions {
		items := make([]wireQueryItem, len(c.Query.Items))
		for j, item := range c.Query.Items {
			items[j] = wireQueryItem{Types: item.Types, Tags: item.Tags}
		}
		var afterHex *string
		if c.After != nil {
			s := hex.EncodeToString(c.After[:])
			afterHex = &s
		}
		wires[i] = wireCondition{QueryItems: items, AfterPosition: afterHex}
	}
	b, err := json.Marshal(wires)
	return string(b), err
}

func marshalQuery(query dcb.Query) (string, error) {
	items := make([]wireQueryItem, len(query.Items))
	for i, item := range query.Items {
		items[i] = wireQueryItem{Types: item.Types, Tags: item.Tags}
	}
	b, err := json.Marshal(items)
	return string(b), err
}

func marshalReadOpts(opts *dcb.ReadOptions) (string, error) {
	wo := wireReadOpts{}
	if opts != nil {
		if opts.Limit > 0 {
			wo.Limit = &opts.Limit
		}
		if opts.After != nil {
			s := hex.EncodeToString(opts.After[:])
			wo.AfterPosition = &s
		}
		wo.Reverse = opts.Reverse
	}
	b, err := json.Marshal(wo)
	return string(b), err
}

// ── Result parsing ────────────────────────────────────────────────────────────

func parseAppendResult(raw string) error {
	var resp wireAppendResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return fmt.Errorf("cstore: parse append response: %w", err)
	}
	if resp.Error != nil {
		if *resp.Error == "condition_failed" {
			return dcb.ErrAppendConditionFailed
		}
		return errors.New(*resp.Error)
	}
	return nil
}

func parseEventsResult(raw string) ([]dcb.StoredEvent, error) {
	var resp wireReadResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("cstore: parse read response: %w", err)
	}
	if resp.Error != nil {
		return nil, errors.New(*resp.Error)
	}

	result := make([]dcb.StoredEvent, len(resp.Events))
	for i, we := range resp.Events {
		posBytes, err := hex.DecodeString(we.Position)
		if err != nil || len(posBytes) != 12 {
			return nil, fmt.Errorf("cstore: invalid position hex %q", we.Position)
		}
		var vs dcb.Versionstamp
		copy(vs[:], posBytes)

		data, err := base64.StdEncoding.DecodeString(we.DataB64)
		if err != nil {
			return nil, fmt.Errorf("cstore: base64 decode data at position %s: %w", we.Position, err)
		}

		result[i] = dcb.StoredEvent{
			Event:    dcb.Event{Type: we.EventType, Tags: we.Tags, Data: data},
			Position: vs,
		}
	}
	return result, nil
}
