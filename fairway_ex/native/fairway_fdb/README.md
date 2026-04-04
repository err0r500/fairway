# fairway_fdb

Rustler NIF adapter that exposes `fairway_fdb_core` to Elixir.
This crate contains no DCB logic — it is purely a protocol bridge between
Erlang terms and the Rust async functions in the core library.

---

## What this crate does

Each NIF function performs three steps:

1. **Deserialise** — JSON string arguments (passed from Elixir via
   `Jason.encode!`) are parsed into `fairway_fdb_core` wire types using
   `serde_json::from_str`.
2. **Execute** — `tokio().block_on(core_fn(...))` runs the async FDB operation
   and blocks the current Rustler `DirtyIo` thread until it completes.
3. **Serialise** — the result is encoded back to Elixir terms (`{:ok, [...]}`,
   `:ok`, `{:error, reason}`) using Rustler's `Encoder` trait.

The `DirtyIo` schedule annotation tells the BEAM to run this NIF on a
dedicated dirty thread pool, not on a scheduler thread. This is required for
any NIF that may block (e.g. on network I/O). Rustler enforces this at compile
time.

---

## NIF signatures

Declared in `Elixir.Fairway.Fdb.Nif`:

```elixir
# Open a database. cluster_file_path and namespace are plain strings.
# Returns {:ok, db_ref} or {:error, reason_string}.
open_db(cluster_file_path :: String.t(), namespace :: String.t())

# Read events. query_json and opts_json are JSON strings.
# Returns {:ok, [{position_hex, type, tags, data_b64}]} or {:error, reason}.
read_events(db_ref, query_json :: String.t(), opts_json :: String.t())

# Append events. events_json and conditions_json are JSON strings.
# Returns :ok | {:error, :condition_failed} | {:error, reason}.
append_events(db_ref, events_json :: String.t(), conditions_json :: String.t())

# Read all events.
# Returns {:ok, [{position_hex, type, tags, data_b64}]} or {:error, reason}.
read_all_events(db_ref)
```

`db_ref` is a Rustler `ResourceArc<DbResource>` — an opaque reference-counted
pointer to a heap-allocated `{Database, namespace}` pair. It is garbage-
collected by the BEAM when no more references exist.

---

## Why JSON strings as arguments instead of Elixir maps?

NIF arguments cross the BEAM ↔ Rust boundary as Erlang terms. Decoding a
nested Elixir map or list of maps into Rust structs requires writing a custom
`Decoder` implementation for each type, which is verbose and fragile.

Using JSON strings means:
- Elixir encodes with `Jason.encode!` (one line, no custom code).
- Rust decodes with `serde_json::from_str` (one line, derived automatically).
- The wire format is the same JSON used by the C FFI, so both callers share
  the same serialisation logic in `fairway_fdb_core`.
- Adding or removing fields is a `serde` annotation change, not a NIF update.

The overhead of JSON encoding/decoding is negligible compared to the FDB
network round-trip that every call involves.

---

## Elixir wrapper: `Fairway.Fdb.Store`

Application code never calls the NIF directly. `Fairway.Fdb.Store` is the
`Fairway.Store` behaviour implementation that:

1. Converts `%QueryItem{}` structs → JSON via `Jason.encode!`.
2. Converts `position` binaries → hex strings (and back) for the NIF.
3. Converts `data` binaries → base64 (and back) for the NIF.
4. Calls the NIF function.
5. Returns results in the format the `Fairway.Store` behaviour specifies.

This means `Fairway.Fdb.Store` is the only place that knows the JSON wire
format; everything above it works with native Elixir types.

---

## Building

Rustler handles the build automatically as part of `mix compile`:

```bash
cd fairway_ex
mix deps.get
mix compile     # triggers cargo build for this crate
```

The compiled `.so` / `.dylib` is placed in `priv/native/fairway_fdb.so`.
In production, `mix release` bundles `priv/native/` inside the release tarball.

For manual builds:
```bash
cd fairway_ex/native
cargo build --release -p fairway_fdb
```
