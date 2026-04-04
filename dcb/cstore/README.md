# dcb/cstore

Go package that implements `dcb.DcbStore` using the Rust
`fairway_fdb_c` shared library via cgo.

This is a drop-in replacement for the native Go FDB store. It allows the Go
application to use the same Rust DCB implementation as the Elixir port, making
the two runtimes storage-compatible and keeping the store logic in a single
canonical codebase.

---

## Prerequisites

### 1. Build the Rust shared library

```bash
# from the repo root
make rust-lib
```

This runs `cargo build --release -p fairway_fdb_c` and produces:
- `fairway_ex/native/target/release/libfairway_fdb_c.so` (Linux)
- `fairway_ex/native/target/release/libfairway_fdb_c.dylib` (macOS)

### 2. Set CGO environment variables

```bash
eval $(make cstore-env)
```

This sets three variables:
```bash
export CGO_CFLAGS="-I/path/to/fairway_ex/native/fairway_fdb_c/include"
export CGO_LDFLAGS="-L/path/to/fairway_ex/native/target/release -lfairway_fdb_c"
export LD_LIBRARY_PATH="/path/to/fairway_ex/native/target/release:$LD_LIBRARY_PATH"
```

`CGO_CFLAGS` tells cgo where to find `fairway_dcb.h`.
`CGO_LDFLAGS` links the Go binary against the Rust shared library.
`LD_LIBRARY_PATH` (or `DYLD_LIBRARY_PATH` on macOS) tells the dynamic linker
where to find the `.so` at runtime.

### 3. FoundationDB client

The Rust library links against the FDB C client (`libfdb_c.so`). Install it:

```bash
# Debian / Ubuntu
apt-get install foundationdb-clients

# macOS
brew install foundationdb
```

---

## Usage

```go
import "github.com/err0r500/fairway/dcb/cstore"

// Open a store (cluster file, namespace)
store, err := cstore.Open("/etc/foundationdb/fdb.cluster", "my_app")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Use it anywhere a dcb.DcbStore is accepted
runner := fairway.NewCommandRunner(store)
err = runner.RunPure(ctx, myCommand)
```

`cstore.Open` returns a `*cstore.Store` which implements the full
`dcb.DcbStore` interface:

| Method | Behaviour |
|---|---|
| `Append(ctx, events, conditions...)` | Calls `fw_append_events`; returns `dcb.ErrAppendConditionFailed` on conflict |
| `Read(ctx, query, opts)` | Calls `fw_read_events`; returns `iter.Seq2[StoredEvent, error]` |
| `ReadAll(ctx)` | Calls `fw_read_all_events` |
| `Database()` | Opens a parallel Go FDB connection for compatibility |
| `Namespace()` | Returns the namespace string |

The `Database()` method exists because `dcb.DcbStore` includes it for
code that needs direct FDB access (e.g. automation queue introspection).
The `cstore.Store` opens its own Go FDB connection to satisfy this; all actual
event read/write goes through the Rust library.

---

## How it works

### cgo boundary

The cgo preamble in `cstore.go` references the compiled Rust library:

```go
// #cgo LDFLAGS: -lfairway_fdb_c
// #cgo CFLAGS: -I${SRCDIR}/../../fairway_ex/native/fairway_fdb_c/include
// #include "fairway_dcb.h"
// #include <stdlib.h>
import "C"
```

`${SRCDIR}` is resolved by cgo to the directory containing `cstore.go`.

### Wire format

Go's `dcb.Event`, `dcb.Query`, `dcb.AppendCondition`, and `dcb.ReadOptions`
are marshalled to JSON before being passed to the C functions, and the JSON
responses are unmarshalled back. No custom binary encoding is needed.

- `dcb.Versionstamp` ([12]byte) → hex string `"0102...0c"` (24 chars)
- `dcb.Event.Data` ([]byte) → base64 string `"eyJ..."` (standard encoding)
- All other fields map directly to JSON

The marshal/unmarshal code is internal to the package and uses standard library
`encoding/json`, `encoding/hex`, and `encoding/base64`.

### Memory management

Every `char*` returned by a `fw_*` function is owned by the Rust allocator.
The Go code immediately converts it to a Go string with `C.GoString(raw)` and
then frees the Rust allocation with `C.fw_free_string(raw)`. The
`defer C.fw_free_string(raw)` pattern ensures this even on error paths.

### Error handling

| Rust response | Go return value |
|---|---|
| `{"ok": true}` | `nil` |
| `{"error": "condition_failed"}` | `dcb.ErrAppendConditionFailed` |
| `{"error": "..."}` | `errors.New(message)` |
| JSON parse failure | `fmt.Errorf("cstore: parse ...: %w", err)` |

`dcb.ErrAppendConditionFailed` is the same sentinel value used by the native
Go store, so `fairway.CommandRunner` retries automatically without any special
casing for the cstore implementation.

---

## Differences from the native Go store

| Aspect | Native Go store | cstore |
|---|---|---|
| FDB access | Go FDB bindings directly | Rust via C FFI |
| Transaction model | Go `db.Transact()` | Rust `tokio().block_on(...)` |
| Streaming reads | Lazy `iter.Seq2` (FDB ranges streamed) | Materialised list then iterated |
| `Database()` | Real Go `fdb.Database` | Separate Go connection |
| CGO dependency | No | Yes — requires Rust toolchain + FDB client at build time |
| Key layout | Identical | Identical |
| DCB semantics | Identical | Identical |

The materialised-list read is the main behavioural difference. For commands
(which read a small, tag-scoped slice of events) this is immaterial. For
large projections over millions of events, use pagination: pass `after_position`
and `limit` in `ReadOptions` and call `Read` in a loop.

---

## Running tests

Tests for this package require the Rust library to be built and the CGO
environment variables to be set:

```bash
# from repo root
make rust-lib
eval $(make cstore-env)
go test ./dcb/cstore/...
```
