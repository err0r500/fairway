/// Rustler NIF entrypoints for the Fairway FDB store.
///
/// All DCB logic lives in `fairway_fdb_core`. This crate is a thin adapter
/// that bridges `fairway_fdb_core` types ↔ Erlang/Elixir terms.
///
/// NIF functions (all DirtyIo via Tokio):
///   open_db(cluster_file_path)               → {:ok, db_ref} | {:error, reason}
///   read_events(db_ref, ns, items, opts)      → {:ok, [stored_event]} | {:error, reason}
///   append_events(db_ref, ns, events, conds)  → :ok | {:error, :condition_failed} | {:error, reason}
///   read_all_events(db_ref, ns)               → {:ok, [stored_event]} | {:error, reason}

use rustler::{Atom, Binary, Encoder, Env, NifResult, ResourceArc, Term};
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
        std::mem::forget(handle);
    });
}

// ── Database resource ─────────────────────────────────────────────────────────

pub struct DbResource {
    pub db: Database,
    pub namespace: String,
}

unsafe impl Send for DbResource {}
unsafe impl Sync for DbResource {}

// ── Atoms ─────────────────────────────────────────────────────────────────────

mod atoms {
    rustler::atoms! { ok, error, condition_failed }
}

// ── NIF: open_db ──────────────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn open_db<'a>(env: Env<'a>, cluster_file_path: String, namespace: String) -> Term<'a> {
    ensure_fdb_network();
    match tokio().block_on(async { Database::from_path(&cluster_file_path) }) {
        Ok(db) => {
            let resource = ResourceArc::new(DbResource { db, namespace });
            (atoms::ok(), resource).encode(env)
        }
        Err(e) => (atoms::error(), format!("{e}")).encode(env),
    }
}

// ── NIF: read_events ──────────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn read_events<'a>(
    env: Env<'a>,
    db_ref: ResourceArc<DbResource>,
    query_json: String,
    opts_json: String,
) -> Term<'a> {
    let items: Vec<QueryItem> = match serde_json::from_str(&query_json) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), format!("query_json: {e}")).encode(env),
    };
    let opts: ReadOpts = match serde_json::from_str(&opts_json) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), format!("opts_json: {e}")).encode(env),
    };

    let result = tokio().block_on(read_events(&db_ref.db, &db_ref.namespace, items, opts));
    match result {
        Ok(events) => {
            // Encode as list of 4-tuples: {position_hex, type, tags, data_b64}
            let encoded: Vec<Term<'a>> = events.into_iter().map(|e| {
                (e.position, e.event_type, e.tags, e.data_b64).encode(env)
            }).collect();
            (atoms::ok(), encoded).encode(env)
        }
        Err(e) => (atoms::error(), e).encode(env),
    }
}

// ── NIF: append_events ────────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn append_events<'a>(
    env: Env<'a>,
    db_ref: ResourceArc<DbResource>,
    events_json: String,
    conditions_json: String,
) -> Term<'a> {
    let events: Vec<EventToAppend> = match serde_json::from_str(&events_json) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), format!("events_json: {e}")).encode(env),
    };
    let conditions: Vec<AppendCondition> = match serde_json::from_str(&conditions_json) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), format!("conditions_json: {e}")).encode(env),
    };

    match tokio().block_on(append_events(&db_ref.db, &db_ref.namespace, events, conditions)) {
        Ok(()) => atoms::ok().encode(env),
        Err(AppendError::ConditionFailed) => (atoms::error(), atoms::condition_failed()).encode(env),
        Err(AppendError::Other(e)) => (atoms::error(), e).encode(env),
    }
}

// ── NIF: read_all_events ──────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn read_all_events<'a>(env: Env<'a>, db_ref: ResourceArc<DbResource>) -> Term<'a> {
    match tokio().block_on(read_all_events(&db_ref.db, &db_ref.namespace)) {
        Ok(events) => {
            let encoded: Vec<Term<'a>> = events.into_iter().map(|e| {
                (e.position, e.event_type, e.tags, e.data_b64).encode(env)
            }).collect();
            (atoms::ok(), encoded).encode(env)
        }
        Err(e) => (atoms::error(), e).encode(env),
    }
}

// ── NIF registration ──────────────────────────────────────────────────────────

fn load(env: Env, _: Term) -> bool {
    rustler::resource!(DbResource, env);
    true
}

rustler::init!(
    "Elixir.Fairway.Fdb.Nif",
    [open_db, read_events, append_events, read_all_events],
    load = load
);
