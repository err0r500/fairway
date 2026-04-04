# fairway_ex/native — Rust workspace

This directory is a Cargo workspace containing the Rust implementation of the
Fairway DCB store. It is compiled in two forms:

| Crate | Output | Consumer |
|---|---|---|
| [`fairway_fdb_core`](fairway_fdb_core/README.md) | `rlib` (static library) | Linked into both crates below |
| [`fairway_fdb`](fairway_fdb/README.md) | `cdylib` | Elixir via Rustler NIF |
| [`fairway_fdb_c`](fairway_fdb_c/README.md) | `cdylib` + `staticlib` | Go via cgo, or any C FFI caller |

The split means all DCB logic — key encoding, tag-tree indexing, k-way merge,
and atomic condition checking — lives in exactly one place (`fairway_fdb_core`)
and is guaranteed to be identical for both callers.

---

## Crate responsibilities

### `fairway_fdb_core` — the shared DCB engine

Contains every algorithm that must run inside an FDB transaction:

- **`keys.rs`** — FDB tuple encoding for all subspace prefixes (`e/`, `t/`,
  `g/`), versionstamp packing, range construction including the
  `range_after_versionstamp` helper used by both reads and condition checks.
- **`tags.rs`** — `generate_all_subsets`: produces every non-empty alphabetically-
  sorted subset of a tag list. Writing all subsets to the tag-tree index on
  append is what makes arbitrary AND-tag queries possible without a full scan.
- **`read.rs`** — `build_query_ranges` (three-path dispatch: types-only,
  types+tags, tags-only with runtime type discovery), k-way min/max heap merge
  over multiple FDB ranges, `read_all_events` for full scans.
- **`append.rs`** — `append_events`: checks all DCB conditions and writes all
  events inside a single `db.Transact()` call.

All wire types (`StoredEvent`, `QueryItem`, `EventToAppend`, `AppendCondition`,
`ReadOpts`) derive `serde::Serialize/Deserialize` so both the NIF and the C FFI
can use JSON as their transport format without any separate encoding layer.

### `fairway_fdb` — Elixir NIF adapter

A thin Rustler wrapper. Each NIF function:
1. Deserialises the JSON string arguments into `fairway_fdb_core` types.
2. Calls `tokio().block_on(core_fn(...))`.
3. Serialises the result back to Elixir terms.

No business logic lives here; it is purely a protocol adapter.

### `fairway_fdb_c` — C FFI export

The same pattern as the NIF adapter but targeting the C ABI. Each `extern "C"`
function takes and returns `*const c_char` JSON strings. A `fw_open_db` /
`fw_close_db` pair manages an opaque `FwDb` handle (a heap-allocated struct
holding the `Database` and namespace string).

This crate is what allows the original Go DCB package to be replaced by the
Rust implementation via cgo — see [`../../dcb/cstore/`](../../dcb/cstore/).

---

## Why Tokio?

The `foundationdb` Rust crate exposes all FDB I/O as `async` futures. Every
NIF call and every C FFI call is a synchronous entry point that needs to drive
those futures to completion before returning. Tokio's `Runtime::block_on()` is
the standard way to do this.

### Why not a single-threaded executor?

FDB's internal network thread communicates with in-flight futures through
channels and wakers. If you use a single-threaded executor (e.g.
`futures::executor::block_on`), both the future being polled and the FDB
waker trying to wake it compete for the same thread, causing a deadlock on the
first real FDB round-trip.

Tokio's multi-threaded executor (`rt-multi-thread`) runs futures on a pool of
OS threads, so the FDB network thread can always wake a future that is parked
on a different thread. This is the only executor configuration that works
reliably with the `foundationdb` crate.

### One runtime per process, shared across all calls

```rust
static TOKIO: OnceCell<Runtime> = OnceCell::new();

fn tokio() -> &'static Runtime {
    TOKIO.get_or_init(|| Runtime::new().expect("failed to create Tokio runtime"))
}
```

`Runtime::new()` spawns a thread pool. Creating one per call would work but
would spin up and tear down threads constantly. The `OnceCell` ensures the
runtime is created at most once per process and reused forever.

Each compiled library (NIF `.so` and C FFI `.so`) has its own private
`TOKIO` static and its own Tokio runtime. They do not share a runtime because
they are separate dynamic libraries loaded into separate address spaces as far
as their statics are concerned. Both work correctly in the same process —
FoundationDB's own network thread is global to the process and is guarded by
the `FDB_NETWORK OnceCell` in the same way.

---

## Building

### Build everything (release)

```bash
cd fairway_ex/native
cargo build --release
```

Produces:
- `target/release/libfairway_fdb.so` (or `.dylib` on macOS) — loaded by Rustler
- `target/release/libfairway_fdb_c.so` + `libfairway_fdb_c.a` — used by Go

Rustler handles the NIF build automatically via `mix compile`. The C library
must be built manually before using the Go cgo bindings (see the root
[`Makefile`](../../Makefile)):

```bash
# from repo root
make rust-lib
```

### Build individual crates

```bash
cargo build --release -p fairway_fdb_core   # just the shared library
cargo build --release -p fairway_fdb        # NIF only
cargo build --release -p fairway_fdb_c      # C FFI only
```

### Run Rust unit tests (no FDB required)

`tags.rs` contains self-contained unit tests for `generate_all_subsets`:

```bash
cargo test -p fairway_fdb_core
```

---

## Key layout (FDB subspaces)

All keys share a `{namespace}` prefix. The layout is identical to the Go
`dcb` package so the two implementations are storage-compatible:

```
{ns}/e/{versionstamp}                          primary event storage
                                               value: tuple(type, tags_tuple, data_bytes)

{ns}/t/{type}/{versionstamp}                   type index
                                               value: ""  (key-only)

{ns}/g/{tag_a}/{tag_b}/.../_e/{type}/{vs}      tag-tree index
                                               value: ""  (key-only)
                                               one entry per non-empty tag subset
```

**Versionstamp** — FDB's globally unique, monotonically increasing 12-byte
value (10-byte transaction version + 2-byte user version). Written using
`SetVersionstampedKey` so FDB fills in the value atomically at commit time.
In the wire format (JSON) positions are hex-encoded 24-character strings.

**Tag subsets** — for an event with tags `[list:x, item:y]`, the tag-tree
index gets entries for `[item:y]`, `[list:x]`, and `[list:x, item:y]`. This
is what enables an efficient AND-query for any subset of tags: you query the
subspace for exactly the subset you want, rather than filtering post-read.
