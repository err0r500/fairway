/// Rustler NIF entrypoints for the Fairway FDB store.
///
/// Exposes four functions to Elixir:
///   open_db(cluster_file_path)                              → {:ok, db_ref} | {:error, reason}
///   read_events(db_ref, namespace, query_items, opts)       → {:ok, [stored_event]} | {:error, reason}
///   append_events(db_ref, namespace, events, conditions)    → :ok | {:error, :condition_failed} | {:error, reason}
///   read_all_events(db_ref, namespace)                      → {:ok, [stored_event]} | {:error, reason}
///
/// All blocking NIF calls run on a Tokio thread pool via rustler::NifResult + spawn_blocking,
/// so they never block the BEAM scheduler.
use rustler::{Atom, Binary, Encoder, Env, NifResult, ResourceArc, Term};
use once_cell::sync::OnceCell;
use tokio::runtime::Runtime;

mod append;
mod keys;
mod read;
mod tags;

use append::{AppendCondition, AppendError, ConditionQueryItem, EventToAppend};
use foundationdb::{api, Database};
use read::{QueryItem, ReadOpts};

// ── Tokio runtime (shared) ───────────────────────────────────────────────────

static TOKIO: OnceCell<Runtime> = OnceCell::new();

fn tokio() -> &'static Runtime {
    TOKIO.get_or_init(|| Runtime::new().expect("failed to create Tokio runtime"))
}

// ── FDB network thread (must be started exactly once) ────────────────────────

static FDB_NETWORK: OnceCell<api::FdbApiHandle> = OnceCell::new();

// ── Database resource ────────────────────────────────────────────────────────

pub struct DbResource {
    pub db: Database,
}

// Safety: Database is Send + Sync in the foundationdb crate
unsafe impl Send for DbResource {}
unsafe impl Sync for DbResource {}

// ── Atoms ────────────────────────────────────────────────────────────────────

mod atoms {
    rustler::atoms! {
        ok,
        error,
        condition_failed,
    }
}

// ── NIF: open_db ─────────────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn open_db<'a>(env: Env<'a>, cluster_file_path: String) -> Term<'a> {
    // Start FDB network thread once
    FDB_NETWORK.get_or_init(|| {
        let handle = api::FdbApiBuilder::default()
            .build()
            .expect("FDB API build failed");
        unsafe { handle.boot().expect("FDB network boot failed") };
        handle
    });

    match tokio().block_on(async {
        Database::from_path(&cluster_file_path)
    }) {
        Ok(db) => {
            let resource = ResourceArc::new(DbResource { db });
            (atoms::ok(), resource).encode(env)
        }
        Err(e) => (atoms::error(), format!("{e}")).encode(env),
    }
}

// ── NIF: read_events ─────────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn read_events<'a>(
    env: Env<'a>,
    db_ref: ResourceArc<DbResource>,
    namespace: String,
    query_items: Vec<rustler::Term<'a>>,
    opts_term: rustler::Term<'a>,
) -> Term<'a> {
    let items: Vec<QueryItem> = match decode_query_items(env, &query_items) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), e).encode(env),
    };

    let opts = match decode_read_opts(opts_term) {
        Ok(o) => o,
        Err(e) => return (atoms::error(), e).encode(env),
    };

    let result = tokio().block_on(read::read_events(&db_ref.db, &namespace, items, opts));

    match result {
        Ok(events) => {
            let encoded: Vec<Term<'a>> = events
                .into_iter()
                .map(|e| encode_stored_event(env, e))
                .collect();
            (atoms::ok(), encoded).encode(env)
        }
        Err(e) => (atoms::error(), e).encode(env),
    }
}

// ── NIF: append_events ───────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn append_events<'a>(
    env: Env<'a>,
    db_ref: ResourceArc<DbResource>,
    namespace: String,
    events_term: Vec<rustler::Term<'a>>,
    conditions_term: Vec<rustler::Term<'a>>,
) -> Term<'a> {
    let events: Vec<EventToAppend> = match decode_events(env, &events_term) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), e).encode(env),
    };

    let conditions: Vec<AppendCondition> = match decode_conditions(env, &conditions_term) {
        Ok(v) => v,
        Err(e) => return (atoms::error(), e).encode(env),
    };

    let result = tokio().block_on(append::append_events(&db_ref.db, &namespace, events, conditions));

    match result {
        Ok(()) => atoms::ok().encode(env),
        Err(AppendError::ConditionFailed) => {
            (atoms::error(), atoms::condition_failed()).encode(env)
        }
        Err(AppendError::Other(e)) => (atoms::error(), e).encode(env),
    }
}

// ── NIF: read_all_events ─────────────────────────────────────────────────────

#[rustler::nif(schedule = "DirtyIo")]
fn read_all_events<'a>(
    env: Env<'a>,
    db_ref: ResourceArc<DbResource>,
    namespace: String,
) -> Term<'a> {
    let result = tokio().block_on(read::read_all_events(&db_ref.db, &namespace));

    match result {
        Ok(events) => {
            let encoded: Vec<Term<'a>> = events
                .into_iter()
                .map(|e| encode_stored_event(env, e))
                .collect();
            (atoms::ok(), encoded).encode(env)
        }
        Err(e) => (atoms::error(), e).encode(env),
    }
}

// ── Term decoding helpers ─────────────────────────────────────────────────────

fn decode_query_items<'a>(env: Env<'a>, terms: &[Term<'a>]) -> Result<Vec<QueryItem>, String> {
    terms.iter().map(|t| {
        let types: Vec<String> = t.map_get("types".encode(env))
            .ok().and_then(|v| v.decode::<Vec<String>>().ok())
            .unwrap_or_default();
        let tags: Vec<String> = t.map_get("tags".encode(env))
            .ok().and_then(|v| v.decode::<Vec<String>>().ok())
            .unwrap_or_default();
        Ok(QueryItem { types, tags })
    }).collect()
}

fn decode_read_opts<'a>(term: Term<'a>) -> Result<ReadOpts, String> {
    let limit: Option<u32> = term.map_get("limit")
        .ok().and_then(|v| v.decode::<u32>().ok());
    let after_bytes: Option<Vec<u8>> = term.map_get("after_position")
        .ok().and_then(|v| v.decode::<Binary>().ok().map(|b| b.as_slice().to_vec()));
    let reverse: bool = term.map_get("reverse")
        .ok().and_then(|v| v.decode::<bool>().ok())
        .unwrap_or(false);

    let after_position = after_bytes
        .as_deref()
        .and_then(keys::bytes_to_versionstamp);

    Ok(ReadOpts { limit, after_position, reverse })
}

fn decode_events<'a>(env: Env<'a>, terms: &[Term<'a>]) -> Result<Vec<EventToAppend>, String> {
    terms.iter().map(|t| {
        let event_type = t.map_get("type".encode(env))
            .ok().and_then(|v| v.decode::<String>().ok())
            .ok_or("event missing 'type' field")?;
        let tags: Vec<String> = t.map_get("tags".encode(env))
            .ok().and_then(|v| v.decode::<Vec<String>>().ok())
            .unwrap_or_default();
        let data: Vec<u8> = t.map_get("data".encode(env))
            .ok().and_then(|v| v.decode::<Binary>().ok().map(|b| b.as_slice().to_vec()))
            .ok_or("event missing 'data' field")?;
        Ok(EventToAppend { event_type, tags, data })
    }).collect()
}

fn decode_conditions<'a>(env: Env<'a>, terms: &[Term<'a>]) -> Result<Vec<AppendCondition>, String> {
    terms.iter().map(|t| {
        let items_term: Vec<Term<'a>> = t.map_get("query_items".encode(env))
            .ok().and_then(|v| v.decode::<Vec<Term<'a>>>().ok())
            .unwrap_or_default();
        let query_items: Vec<ConditionQueryItem> = items_term.iter().map(|it| {
            let types: Vec<String> = it.map_get("types".encode(env))
                .ok().and_then(|v| v.decode::<Vec<String>>().ok())
                .unwrap_or_default();
            let tags: Vec<String> = it.map_get("tags".encode(env))
                .ok().and_then(|v| v.decode::<Vec<String>>().ok())
                .unwrap_or_default();
            ConditionQueryItem { types, tags }
        }).collect();

        let after_position = t.map_get("after_position".encode(env))
            .ok()
            .and_then(|v| v.decode::<Binary>().ok())
            .and_then(|b| keys::bytes_to_versionstamp(b.as_slice()));

        Ok(AppendCondition { query_items, after_position })
    }).collect()
}

// ── Term encoding helpers ─────────────────────────────────────────────────────

fn encode_stored_event<'a>(env: Env<'a>, e: read::StoredEvent) -> Term<'a> {
    let position_bytes = keys::versionstamp_to_bytes(&e.position);
    let tags_encoded: Vec<Term<'a>> = e.tags.iter().map(|t| t.encode(env)).collect();
    // Return as 4-tuple: {position_binary, type_string, tags_list, data_binary}
    (
        position_bytes.encode(env),
        e.event_type.encode(env),
        tags_encoded.encode(env),
        e.data.encode(env),
    ).encode(env)
}

// ── NIF registration ─────────────────────────────────────────────────────────

fn load(env: Env, _: Term) -> bool {
    rustler::resource!(DbResource, env);
    true
}

rustler::init!(
    "Elixir.Fairway.Fdb.Nif",
    [open_db, read_events, append_events, read_all_events],
    load = load
);
