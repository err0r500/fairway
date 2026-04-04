/// Append path: DCB condition check + atomic write.
///
/// Mirrors Go's append.go: everything happens in a single db.Transact call.
/// 1. For each condition: check if any event matching query_items exists after after_position.
///    If yes → return ErrAppendConditionFailed (Elixir: {:error, :condition_failed}).
/// 2. If all conditions pass → write all events with versionstamped keys.
use foundationdb::{Database, KeySelector, RangeOption};
use foundationdb::options::MutationType;
use foundationdb::tuple::Versionstamp;

use crate::keys::{
    encode_event_value, event_key_incomplete, type_index_key_incomplete, tag_index_key_incomplete,
    type_index_prefix, tag_index_type_prefix, tag_events_marker_prefix,
    prefix_range, range_after_versionstamp,
    extract_type_from_tag_key,
};
use crate::tags::{generate_all_subsets, sorted_tags};

/// An event to append.
#[derive(Debug)]
pub struct EventToAppend {
    pub event_type: String,
    pub tags: Vec<String>,
    pub data: Vec<u8>,
}

/// A single query item used in an append condition.
#[derive(Debug, Clone)]
pub struct ConditionQueryItem {
    pub types: Vec<String>,
    pub tags: Vec<String>,
}

/// An append condition: "no events matching query_items exist after after_position".
#[derive(Debug)]
pub struct AppendCondition {
    pub query_items: Vec<ConditionQueryItem>,
    pub after_position: Option<Versionstamp>,
}

/// Error variants from append_events.
#[derive(Debug)]
pub enum AppendError {
    ConditionFailed,
    Other(String),
}

/// Append events with optional DCB conditions inside a single FDB transaction.
pub async fn append_events(
    db: &Database,
    namespace: &str,
    events: Vec<EventToAppend>,
    conditions: Vec<AppendCondition>,
) -> Result<(), AppendError> {
    if events.is_empty() {
        return Err(AppendError::Other("events list is empty".into()));
    }

    let namespace = namespace.to_string();

    // FDB transactions are synchronous within the foundationdb crate's run() loop.
    // We build and commit a single transaction.
    let trx = db.create_trx().map_err(|e| AppendError::Other(format!("create_trx: {e}")))?;

    // Step 1: Check all conditions (reads within the same transaction)
    for condition in &conditions {
        let exists = condition_exists(&trx, &namespace, condition).await
            .map_err(AppendError::Other)?;
        if exists {
            return Err(AppendError::ConditionFailed);
        }
    }

    // Step 2: Write all events
    for (batch_idx, event) in events.iter().enumerate() {
        write_event(&trx, &namespace, event, batch_idx as u16)
            .map_err(AppendError::Other)?;
    }

    // Step 3: Commit
    trx.commit().await.map_err(|e| AppendError::Other(format!("commit: {e}")))?;

    Ok(())
}

/// Check whether any events matching the condition's query_items exist after after_position.
async fn condition_exists(
    trx: &foundationdb::Transaction,
    namespace: &str,
    condition: &AppendCondition,
) -> Result<bool, String> {
    for item in &condition.query_items {
        let exists = query_item_exists(trx, namespace, item, condition.after_position.as_ref()).await?;
        if exists {
            return Ok(true); // OR semantics
        }
    }
    Ok(false)
}

/// Check if any events match a single query item (LIMIT 1 scan).
async fn query_item_exists(
    trx: &foundationdb::Transaction,
    namespace: &str,
    item: &ConditionQueryItem,
    after: Option<&Versionstamp>,
) -> Result<bool, String> {
    let has_types = !item.types.is_empty();
    let has_tags = !item.tags.is_empty();

    if !has_types && !has_tags {
        return Err("condition query item must have at least one type or tag".into());
    }

    let ranges: Vec<(Vec<u8>, Vec<u8>)> = if has_types && !has_tags {
        // Type-only
        item.types.iter().map(|t| {
            let prefix = type_index_prefix(namespace, t);
            if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            }
        }).collect()
    } else {
        // Tags (with or without types)
        let mut stags = item.tags.clone();
        stags.sort();
        let srefs: Vec<&str> = stags.iter().map(|s| s.as_str()).collect();

        let types: Vec<String> = if has_types {
            item.types.clone()
        } else {
            discover_types(trx, namespace, &srefs).await?
        };

        types.iter().map(|t| {
            let prefix = tag_index_type_prefix(namespace, &srefs, t);
            if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            }
        }).collect()
    };

    for (begin, end) in ranges {
        let range_opt = RangeOption {
            begin: KeySelector::first_greater_or_equal(begin),
            end: KeySelector::first_greater_or_equal(end),
            limit: Some(1),
            reverse: false,
            ..RangeOption::default()
        };
        let kvs = trx.get_range(&range_opt, 1, false)
            .await
            .map_err(|e| format!("condition check get_range: {e}"))?;
        if !kvs.is_empty() {
            return Ok(true);
        }
    }

    Ok(false)
}

/// Discover event types under a tag combination's _e subspace.
async fn discover_types(
    trx: &foundationdb::Transaction,
    namespace: &str,
    sorted_tags: &[&str],
) -> Result<Vec<String>, String> {
    let marker_prefix = tag_events_marker_prefix(namespace, sorted_tags);
    let (begin, end) = prefix_range(&marker_prefix);

    let range_opt = RangeOption {
        begin: KeySelector::first_greater_or_equal(begin),
        end: KeySelector::first_greater_or_equal(end),
        limit: None,
        reverse: false,
        ..RangeOption::default()
    };

    let kvs = trx.get_range(&range_opt, 1, false)
        .await
        .map_err(|e| format!("discover_types get_range: {e}"))?;

    let mut seen = std::collections::HashSet::new();
    let mut types = Vec::new();
    for kv in kvs.iter() {
        if let Some(t) = extract_type_from_tag_key(kv.key()) {
            if seen.insert(t.clone()) {
                types.push(t);
            }
        }
    }

    Ok(types)
}

/// Write a single event with all indexes using versionstamped keys.
fn write_event(
    trx: &foundationdb::Transaction,
    namespace: &str,
    event: &EventToAppend,
    batch_index: u16,
) -> Result<(), String> {
    // 1. Primary event key: {namespace}/e/{versionstamp} → encoded_value
    let (event_key, vs_offset) = event_key_incomplete(namespace, batch_index);
    let event_value = encode_event_value(&event.event_type, &event.tags, &event.data);
    trx.atomic_op(&event_key, &event_value, MutationType::SetVersionstampedKey);
    // Note: SetVersionstampedKey requires the versionstamp offset to be appended to the key.
    // The foundationdb Rust crate handles this via pack_with_versionstamp_offset above.

    // 2. Type index: {namespace}/t/{type}/{versionstamp} → ""
    let (type_key, _) = type_index_key_incomplete(namespace, &event.event_type, batch_index);
    trx.atomic_op(&type_key, &[], MutationType::SetVersionstampedKey);

    // 3. Tag tree: for each non-empty subset of sorted tags
    let subsets = generate_all_subsets(&event.tags);
    for subset in &subsets {
        let subset_refs: Vec<&str> = subset.iter().map(|s| s.as_str()).collect();
        let (tag_key, _) = tag_index_key_incomplete(namespace, &subset_refs, &event.event_type, batch_index);
        trx.atomic_op(&tag_key, &[], MutationType::SetVersionstampedKey);
    }

    Ok(())
}
