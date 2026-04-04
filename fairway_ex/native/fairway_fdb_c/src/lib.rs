//! C FFI for the Fairway DCB store.
//!
//! Exposes four functions callable from any language with C FFI (Go via cgo, Python via ctypes, etc.):
//!
//!   fw_open_db(cluster_file)  → FwDb handle or NULL
//!   fw_read_events(...)       → JSON string (caller frees with fw_free_string)
//!   fw_append_events(...)     → JSON string
//!   fw_read_all_events(...)   → JSON string
//!   fw_close_db(db)
//!   fw_free_string(s)
//!
//! ## JSON formats
//!
//! ### read_events / read_all_events response
//! ```json
//! {"events": [{"position": "aabbcc...", "type": "ListCreated", "tags": ["list:x"], "data_b64": "..."}]}
//! {"error": "some error message"}
//! ```
//!
//! ### read_events query_json
//! ```json
//! [{"types": ["ListCreated"], "tags": ["list:x"]}]
//! ```
//!
//! ### read_events opts_json
//! ```json
//! {"limit": 100, "after_position": "aabbcc...", "reverse": false}
//! ```
//!
//! ### append_events events_json
//! ```json
//! [{"type": "ListCreated", "tags": ["list:x"], "data_b64": "eyJsaXN0X2lkIjoieCJ9"}]
//! ```
//!
//! ### append_events conditions_json
//! ```json
//! [{"query_items": [{"types": ["ListCreated"], "tags": ["list:x"]}], "after_position": null}]
//! ```
//!
//! ### append_events response
//! ```json
//! {"ok": true}
//! {"error": "condition_failed"}
//! {"error": "some other error"}
//! ```

use std::ffi::{CStr, CString};
use std::os::raw::c_char;

use once_cell::sync::OnceCell;
use tokio::runtime::Runtime;
use foundationdb::{api, Database};

use fairway_fdb_core::{
    append_events, read_events, read_all_events,
    AppendCondition, AppendError, EventToAppend, QueryItem, ReadOpts,
};

// ── Tokio runtime ─────────────────────────────────────────────────────────────

static TOKIO: OnceCell<Runtime> = OnceCell::new();

fn tokio() -> &'static Runtime {
    TOKIO.get_or_init(|| Runtime::new().expect("failed to create Tokio runtime"))
}

// ── FDB network ───────────────────────────────────────────────────────────────

static FDB_NETWORK: OnceCell<()> = OnceCell::new();

fn ensure_fdb_network() {
    FDB_NETWORK.get_or_init(|| {
        let handle = api::FdbApiBuilder::default()
            .build()
            .expect("FDB API build failed");
        unsafe { handle.boot().expect("FDB network boot failed") };
        std::mem::forget(handle); // keep network alive for process lifetime
    });
}

// ── Opaque database handle ────────────────────────────────────────────────────

pub struct FwDbInner {
    db: Database,
    namespace: String,
}

/// Opaque handle returned to callers. Cast to/from `*mut FwDbInner`.
pub type FwDb = *mut FwDbInner;

// ── Helpers ───────────────────────────────────────────────────────────────────

fn to_cstring(s: String) -> *mut c_char {
    CString::new(s).unwrap_or_default().into_raw()
}

fn ok_json(value: serde_json::Value) -> *mut c_char {
    to_cstring(value.to_string())
}

fn err_json(msg: &str) -> *mut c_char {
    to_cstring(serde_json::json!({"error": msg}).to_string())
}

unsafe fn cstr(ptr: *const c_char) -> Option<&'static str> {
    if ptr.is_null() {
        None
    } else {
        CStr::from_ptr(ptr).to_str().ok()
    }
}

// ── Public C API ──────────────────────────────────────────────────────────────

/// Open an FDB database.
///
/// `cluster_file_path` may be NULL to use the default cluster file
/// ($FDB_CLUSTER_FILE or /etc/foundationdb/fdb.cluster).
/// `namespace` may be NULL to use "fairway".
///
/// Returns an opaque `FwDb` handle, or NULL on failure.
/// Call `fw_close_db` when done.
#[no_mangle]
pub extern "C" fn fw_open_db(
    cluster_file_path: *const c_char,
    namespace: *const c_char,
) -> FwDb {
    ensure_fdb_network();

    let path = unsafe { cstr(cluster_file_path) }
        .unwrap_or("/etc/foundationdb/fdb.cluster");
    let ns = unsafe { cstr(namespace) }.unwrap_or("fairway").to_string();

    match tokio().block_on(async { Database::from_path(path) }) {
        Ok(db) => {
            let inner = Box::new(FwDbInner { db, namespace: ns });
            Box::into_raw(inner)
        }
        Err(_) => std::ptr::null_mut(),
    }
}

/// Close a database handle opened with `fw_open_db`.
#[no_mangle]
pub extern "C" fn fw_close_db(db: FwDb) {
    if !db.is_null() {
        unsafe { drop(Box::from_raw(db)) };
    }
}

/// Read events matching query_items.
///
/// `query_json`  — JSON array of query items: `[{"types":[...],"tags":[...]}]`
/// `opts_json`   — JSON object: `{"limit":N,"after_position":"hex24","reverse":false}`
///                 Pass `"{}"` for defaults (no limit, from start, forward).
///
/// Returns a JSON string. Caller must free with `fw_free_string`.
/// On success: `{"events":[...]}`
/// On error:   `{"error":"..."}`
#[no_mangle]
pub extern "C" fn fw_read_events(
    db: FwDb,
    query_json: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    if db.is_null() {
        return err_json("null db handle");
    }
    let inner = unsafe { &*db };

    let query_str = match unsafe { cstr(query_json) } {
        Some(s) => s,
        None => return err_json("null query_json"),
    };
    let opts_str = unsafe { cstr(opts_json) }.unwrap_or("{}");

    let items: Vec<QueryItem> = match serde_json::from_str(query_str) {
        Ok(v) => v,
        Err(e) => return err_json(&format!("query_json parse: {e}")),
    };
    let opts: ReadOpts = match serde_json::from_str(opts_str) {
        Ok(v) => v,
        Err(e) => return err_json(&format!("opts_json parse: {e}")),
    };

    match tokio().block_on(read_events(&inner.db, &inner.namespace, items, opts)) {
        Ok(events) => ok_json(serde_json::json!({"events": events})),
        Err(e) => err_json(&e),
    }
}

/// Append events with optional DCB conditions.
///
/// `events_json`     — JSON array: `[{"type":"...","tags":[...],"data_b64":"..."}]`
/// `conditions_json` — JSON array: `[{"query_items":[...],"after_position":"hex24"|null}]`
///                     Pass `"[]"` for unconditional append.
///
/// Returns a JSON string. Caller must free with `fw_free_string`.
/// On success:           `{"ok":true}`
/// On condition failure: `{"error":"condition_failed"}`
/// On other error:       `{"error":"..."}`
#[no_mangle]
pub extern "C" fn fw_append_events(
    db: FwDb,
    events_json: *const c_char,
    conditions_json: *const c_char,
) -> *mut c_char {
    if db.is_null() {
        return err_json("null db handle");
    }
    let inner = unsafe { &*db };

    let events_str = match unsafe { cstr(events_json) } {
        Some(s) => s,
        None => return err_json("null events_json"),
    };
    let conds_str = unsafe { cstr(conditions_json) }.unwrap_or("[]");

    let events: Vec<EventToAppend> = match serde_json::from_str(events_str) {
        Ok(v) => v,
        Err(e) => return err_json(&format!("events_json parse: {e}")),
    };
    let conditions: Vec<AppendCondition> = match serde_json::from_str(conds_str) {
        Ok(v) => v,
        Err(e) => return err_json(&format!("conditions_json parse: {e}")),
    };

    match tokio().block_on(append_events(&inner.db, &inner.namespace, events, conditions)) {
        Ok(()) => ok_json(serde_json::json!({"ok": true})),
        Err(AppendError::ConditionFailed) => err_json("condition_failed"),
        Err(AppendError::Other(e)) => err_json(&e),
    }
}

/// Read all events in the namespace in versionstamp order.
///
/// Returns a JSON string. Caller must free with `fw_free_string`.
/// On success: `{"events":[...]}`
/// On error:   `{"error":"..."}`
#[no_mangle]
pub extern "C" fn fw_read_all_events(db: FwDb) -> *mut c_char {
    if db.is_null() {
        return err_json("null db handle");
    }
    let inner = unsafe { &*db };

    match tokio().block_on(read_all_events(&inner.db, &inner.namespace)) {
        Ok(events) => ok_json(serde_json::json!({"events": events})),
        Err(e) => err_json(&e),
    }
}

/// Free a string returned by any `fw_*` function.
#[no_mangle]
pub extern "C" fn fw_free_string(s: *mut c_char) {
    if !s.is_null() {
        unsafe { drop(CString::from_raw(s)) };
    }
}
