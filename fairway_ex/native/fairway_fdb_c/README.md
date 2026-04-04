# fairway_fdb_c

C FFI export of the Fairway DCB store. Compiles to both a shared library
(`libfairway_fdb_c.so` / `.dylib`) and a static archive
(`libfairway_fdb_c.a`) so it can be linked by any language with C FFI support.

Primary consumer: the Go `dcb/cstore` package via cgo.
The same library can be used from Python (ctypes/cffi), Ruby (fiddle), or any
other runtime that can call C.

---

## C API

The header is at [`include/fairway_dcb.h`](include/fairway_dcb.h).

```c
// Open / close
FwDb   fw_open_db(const char* cluster_file_path, const char* namespace_);
void   fw_close_db(FwDb db);

// Read events matching a query
char*  fw_read_events(FwDb db, const char* query_json, const char* opts_json);

// Append events with optional DCB conditions
char*  fw_append_events(FwDb db, const char* events_json, const char* conditions_json);

// Read all events in versionstamp order
char*  fw_read_all_events(FwDb db);

// Free any string returned by the above
void   fw_free_string(char* s);
```

### Ownership

Every `char*` returned by a `fw_*` function is heap-allocated by Rust and must
be freed exactly once with `fw_free_string()`. Never pass the pointer to
`free()` directly — the allocator in the Rust shared library may differ from
the one in the C caller.

### Thread safety

`FwDb` handles are safe to use from multiple threads concurrently. The FDB
client and Tokio runtime are both internally thread-safe.

---

## JSON formats

All data crosses the C boundary as UTF-8 JSON strings. This keeps the API
language-agnostic and easy to inspect during debugging.

### Positions

Versionstamp positions are lowercase hex strings, 24 characters (12 bytes):
```
"0102030405060708090a0b0c"
```

### Event data

Event `data` is base64-encoded (standard alphabet, no line breaks) because it
is arbitrary bytes (typically application-level JSON).

### `fw_read_events` — query_json

```json
[
  {"types": ["ListCreated", "ItemAdded"], "tags": ["list:x"]},
  {"types": [],                           "tags": ["user:y"]}
]
```

Each object is one query item. Items are OR'd together. Within an item:
- `types` are OR'd (empty = match all types)
- `tags` are AND'd (empty = match all tags)

### `fw_read_events` — opts_json

```json
{
  "limit": 100,
  "after_position": "0102030405060708090a0b0c",
  "reverse": false
}
```

All fields are optional. Pass `"{}"` for defaults (no limit, from the
beginning, forward order).

### `fw_read_events` / `fw_read_all_events` — response

```json
{
  "events": [
    {
      "position": "0102030405060708090a0b0c",
      "type":     "ListCreated",
      "tags":     ["list:x"],
      "data_b64": "eyJsaXN0X2lkIjoieCIsIm5hbWUiOiJTaG9wcGluZyJ9"
    }
  ]
}
```

On error:
```json
{"error": "some error message"}
```

### `fw_append_events` — events_json

```json
[
  {
    "type":     "ListCreated",
    "tags":     ["list:x"],
    "data_b64": "eyJsaXN0X2lkIjoieCIsIm5hbWUiOiJTaG9wcGluZyJ9"
  }
]
```

### `fw_append_events` — conditions_json

```json
[
  {
    "query_items": [
      {"types": ["ListCreated"], "tags": ["list:x"]}
    ],
    "after_position": null
  }
]
```

Pass `"[]"` for an unconditional append.

`"after_position": null` means "check from the very beginning of the log".
`"after_position": "hex24"` means "only conflict if a matching event exists
*after* this position" — the position is the highest versionstamp the command
saw when it read. If nothing was read, pass `null`.

### `fw_append_events` — response

```json
{"ok": true}
```
```json
{"error": "condition_failed"}
```
```json
{"error": "some other error message"}
```

---

## Building

### From the repo root

```bash
make rust-lib          # release build
make rust-lib-debug    # debug build (faster compile)
```

### Manually

```bash
cd fairway_ex/native
cargo build --release -p fairway_fdb_c
```

Output files:

| File | Use |
|---|---|
| `target/release/libfairway_fdb_c.so` | Dynamic linking (Linux) |
| `target/release/libfairway_fdb_c.dylib` | Dynamic linking (macOS) |
| `target/release/libfairway_fdb_c.a` | Static linking |

The FDB C client (`libfdb_c.so`) must be present at runtime when using the
shared library. Install it via:
```bash
# Debian / Ubuntu
apt-get install foundationdb-clients

# macOS
brew install foundationdb
```

---

## Example: C caller

```c
#include "include/fairway_dcb.h"
#include <stdio.h>

int main(void) {
    FwDb db = fw_open_db(NULL, "my_app");
    if (!db) { fprintf(stderr, "failed to open db\n"); return 1; }

    // Append an event unconditionally
    char* result = fw_append_events(
        db,
        "[{\"type\":\"ListCreated\",\"tags\":[\"list:x\"],\"data_b64\":\"e30=\"}]",
        "[]"
    );
    printf("append: %s\n", result);
    fw_free_string(result);

    // Read it back
    result = fw_read_events(db,
        "[{\"types\":[\"ListCreated\"],\"tags\":[\"list:x\"]}]",
        "{}"
    );
    printf("read: %s\n", result);
    fw_free_string(result);

    fw_close_db(db);
    return 0;
}
```

Compile:
```bash
gcc example.c \
  -I include \
  -L ../../target/release \
  -lfairway_fdb_c \
  -Wl,-rpath,../../target/release \
  -o example
```

---

## Internal structure (`src/lib.rs`)

The source is a single file. Each `extern "C"` function:

1. Guards against null `FwDb` / null string pointers.
2. Deserialises JSON input with `serde_json::from_str`.
3. Calls `tokio().block_on(fairway_fdb_core::...)` to run the async FDB
   operation on the Tokio thread pool and wait for the result.
4. Serialises the result to JSON with `serde_json::json!(...)`.
5. Returns a heap-allocated `CString` via `CString::into_raw()`.

`fw_free_string` reclaims that memory with `CString::from_raw()`.

The `FwDb` opaque type is a `Box<FwDbInner>` leaked to a raw pointer on
`fw_open_db` and reclaimed on `fw_close_db`. All Rust ownership rules are
respected; the raw pointer is only an implementation detail hidden behind the
C API.
