# fairway_ex

Elixir port of [Fairway](../README.md) — a framework for building backends from
small, self-contained modules that communicate through a shared append-only event
log, using [Dynamic Consistency Boundaries (DCB)](https://dcb.events) for
lock-free optimistic concurrency.

The storage backend is **FoundationDB**, accessed through a **thick Rustler NIF**
that owns the entire store layer. A **C FFI export** of the same Rust code lets
the original Go codebase import the same implementation via cgo.

---

## Why an Elixir port?

The Go version and this Elixir port share the same DCB correctness model and the
same FoundationDB key layout. The Elixir layer adds OTP supervision trees,
pattern-matched event dispatch, and the BEAM's process model for automations.
The Rust layer — shared between both — ensures a single canonical implementation
of the store: key encoding, tag-tree indexing, k-way merge reads, and atomic
condition checks all live in one place.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  Elixir application layer                                        │
│                                                                  │
│  Fairway.CommandRunner   ──┐                                     │
│  Fairway.View            ──┤─── Fairway.ReadAppender             │
│  Fairway.Automation.*    ──┘         │                           │
│                                      ▼                           │
│                            Fairway.Store (behaviour)             │
│                           ┌──────────┴──────────┐               │
│                  Fairway.Fdb.Store       Fairway.Testing.Store   │
│                  (production)            (ETS, tests only)       │
│                       │                                          │
└───────────────────────┼──────────────────────────────────────────┘
                        │  JSON wire format (serde)
┌───────────────────────┼──────────────────────────────────────────┐
│  Rust NIF             │                                          │
│                  fairway_fdb  (Rustler cdylib)                   │
│                       │                                          │
│                  fairway_fdb_core  (shared lib)                  │
│                  ┌────┴─────────────────────┐                    │
│                  keys.rs   tags.rs   read.rs  append.rs          │
│                                      │                           │
└──────────────────────────────────────┼───────────────────────────┘
                                       │  foundationdb Rust crate
                                 FoundationDB C client
                                       │
                                 FoundationDB cluster
```

The same `fairway_fdb_core` library is also compiled as a C shared library
(`fairway_fdb_c`) so the Go codebase can import it via cgo — see
[`../dcb/cstore/`](../dcb/cstore/README.md).

---

## Project structure

```
fairway_ex/
├── mix.exs                          # Mix project (Elixir + Rustler)
├── lib/fairway/
│   ├── application.ex               # OTP Application, starts Registry
│   ├── store.ex                     # Behaviour: read_events, append_events, read_all_events
│   ├── event.ex                     # Event struct + EventData behaviour
│   ├── query.ex                     # Query / QueryItem builder DSL
│   ├── read_appender.ex             # Immutable command state (read tracking + condition build)
│   ├── command_runner.ex            # Retry loop on {:error, :condition_failed}
│   ├── view.ex                      # Read-only projection fold
│   ├── registry.ex                  # ETS: type_name string → module atom
│   ├── slice.ex                     # `use Fairway.Slice` macro
│   ├── fdb/
│   │   ├── nif.ex                   # Rustler NIF declarations
│   │   └── store.ex                 # Fairway.Store impl → NIF
│   └── testing/
│       └── store.ex                 # In-memory ETS store (no FDB, for tests)
├── native/                          # Rust workspace — see native/README.md
│   ├── Cargo.toml                   # Workspace root
│   ├── fairway_fdb_core/            # Shared DCB logic
│   ├── fairway_fdb/                 # Elixir NIF (cdylib)
│   └── fairway_fdb_c/               # C FFI for Go (cdylib + staticlib)
└── test/
    ├── support/test_events.ex       # Shared test event/command fixtures
    └── fairway/
        ├── testing_store_test.exs   # Store roundtrip, DCB conditions
        └── command_runner_test.exs  # Commands, retry, views
```

---

## Core concepts

### Events — plain structs, not Ecto schemas

Events are immutable facts. They are represented as plain Elixir structs
implementing the `Fairway.EventData` behaviour:

```elixir
defmodule MyApp.Events.ListCreated do
  @behaviour Fairway.EventData
  @derive Jason.Encoder
  defstruct [:list_id, :name]

  @impl true
  def tags(%{list_id: id}), do: ["list:#{id}"]

  @impl true
  def type_name, do: "ListCreated"   # stable string key in FDB
end
```

`tags/1` defines the DCB scope — the narrower the tags, the less contention
between concurrent commands. `type_name/0` is the stable string stored in FDB
(rename the module freely; the string must stay stable).

Ecto embedded schemas are deliberately not used here. Events are already-valid
facts being written or read; changeset validation belongs at the HTTP boundary,
not in domain events.

### Commands — read, decide, append

A command reads events to derive current state, makes a decision, then appends
new events. The `Fairway.ReadAppender` struct threads through the function,
tracking the highest versionstamp seen per read so the append can include DCB
conditions automatically:

```elixir
defmodule MyApp.Commands.CreateList do
  @behaviour Fairway.Command

  def run(ra, %{list_id: id, name: name}) do
    query = Fairway.Query.new([
      Fairway.QueryItem.new(types: [MyApp.Events.ListCreated], tags: ["list:#{id}"])
    ])

    # fold_events returns {updated_ra, acc}
    {ra, exists} =
      Fairway.ReadAppender.fold_events(ra, query, false, fn
        %{data: %MyApp.Events.ListCreated{}}, _ -> {false, true}
        _, acc -> {true, acc}
      end)

    if exists do
      {:error, :already_exists}
    else
      # append builds AppendCondition from ra.reads automatically
      Fairway.ReadAppender.append_events(ra, [
        Fairway.Event.new(%MyApp.Events.ListCreated{list_id: id, name: name})
      ])
    end
  end
end
```

`CommandRunner` wraps the retry loop:

```elixir
runner = Fairway.CommandRunner.new(store, Fairway.Fdb.Store)
:ok = Fairway.CommandRunner.run_pure(runner, MyApp.Commands.CreateList,
        %{list_id: "x", name: "Shopping"})
```

On `{:error, :condition_failed}` (a concurrent write beat us to it), the runner
retries from scratch — new read, new decision, new append — up to `max_attempts`
times with exponential backoff.

### Views — read-only projections

```elixir
query = Fairway.Query.new([
  Fairway.QueryItem.new(
    types: [MyApp.Events.ListCreated, MyApp.Events.ItemAdded],
    tags: ["list:#{list_id}"]
  )
])

{:ok, result} = Fairway.View.read_events(store, Fairway.Fdb.Store, query,
  %{name: nil, items: []},
  fn
    %{data: %MyApp.Events.ListCreated{name: n}}, acc -> %{acc | name: n}
    %{data: %MyApp.Events.ItemAdded{name: i}}, acc -> Map.update!(acc, :items, &[i | &1])
    _, acc -> acc
  end)
```

### Slices — self-contained feature modules

Each feature is a module that registers itself at application start using the
`use Fairway.Slice` macro. Slices never import each other; the only shared
artifact between them is the event type name string.

---

## Getting started

### Prerequisites

- Elixir 1.16+ / OTP 26+
- Rust 1.75+ with `cargo`
- FoundationDB 7.1 client libraries installed
  (`foundationdb-clients` package or `brew install foundationdb`)

### Build

```bash
# 1. Install Elixir dependencies (triggers Rustler to compile the NIF)
cd fairway_ex
mix deps.get
mix compile
```

The `mix compile` step invokes `cargo build` for `fairway_fdb` automatically
via Rustler. The compiled `.so` / `.dylib` is placed in `priv/native/`.

### Run tests (no FDB required)

All unit tests use `Fairway.Testing.Store` (an in-memory ETS store) and run
without a live FDB cluster:

```bash
mix test
```

### Run integration tests (FDB required)

```bash
FDB_CLUSTER_FILE=/etc/foundationdb/fdb.cluster mix test --include integration
```

---

## Why Tokio?

The `foundationdb` Rust crate models all FDB operations — range reads,
`get()`, transaction commits — as `async` Rust futures. To drive those futures
to completion from a synchronous call site (either a Rustler NIF call or a C
FFI call), an async runtime is required.

**Tokio** is the de-facto standard Rust async runtime and the only one the
`foundationdb` crate is tested against. It provides:

- A **multi-threaded executor** needed because FDB's internal network thread
  communicates with futures via channels; a single-threaded executor like
  `futures::executor::block_on` deadlocks when FDB's network thread tries to
  wake a future that is being polled on the same thread.
- **`Runtime::block_on()`** — used at every NIF/C entry point to bridge the
  async Rust world back to a synchronous call. This means each NIF call blocks
  its own OS thread (a Rustler `DirtyIo` thread, never a BEAM scheduler thread)
  until the FDB transaction completes, then returns.
- **Thread safety** — Tokio's `Runtime` is `Send + Sync`, making it safe to
  store in a `static OnceCell<Runtime>` shared across all NIF calls.

The single shared `Runtime` is initialised once per process via `OnceCell` and
reused for all subsequent calls. Creating a new runtime per call would be
correct but wasteful (each `Runtime::new()` spawns OS threads).

A lighter alternative (`smol`, `async-std`) would also work in principle, but
would require patching the `foundationdb` crate's internal waker assumptions
and is not tested upstream.

---

## See also

- [`native/README.md`](native/README.md) — Rust workspace, three crates, build guide
- [`../dcb/cstore/README.md`](../dcb/cstore/README.md) — Go cgo bindings
- [`../README.md`](../README.md) — Original Go Fairway framework
