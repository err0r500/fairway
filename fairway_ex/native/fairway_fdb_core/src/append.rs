/// Append path: DCB condition check + atomic write in a single FDB Transaction.
use foundationdb::{Database, KeySelector, RangeOption, Transaction};
use foundationdb::options::MutationType;
use foundationdb::tuple::Versionstamp;

use crate::keys::{
    encode_event_value, event_key_incomplete, type_index_key_incomplete, tag_index_key_incomplete,
    type_index_prefix, tag_index_type_prefix, tag_events_marker_prefix,
    prefix_range, range_after_versionstamp, extract_type_from_tag_key,
    bytes_to_versionstamp,
};
use crate::tags::generate_all_subsets;
use crate::read::{QueryItem, discover_types_in_tag_subspace};

/// An event to append.
#[derive(Debug, serde::Deserialize, serde::Serialize)]
pub struct EventToAppend {
    #[serde(rename = "type")]
    pub event_type: String,
    #[serde(default)]
    pub tags: Vec<String>,
    /// Base64-encoded event data.
    pub data_b64: String,
}

impl EventToAppend {
    pub fn data_bytes(&self) -> Result<Vec<u8>, String> {
        base64::Engine::decode(&base64::engine::general_purpose::STANDARD, &self.data_b64)
            .map_err(|e| format!("base64 decode: {e}"))
    }
}

/// An append condition.
#[derive(Debug, serde::Deserialize, serde::Serialize)]
pub struct AppendCondition {
    pub query_items: Vec<QueryItem>,
    /// Hex-encoded 12-byte versionstamp or null.
    pub after_position: Option<String>,
}

impl AppendCondition {
    pub fn after_versionstamp(&self) -> Option<Versionstamp> {
        self.after_position.as_deref().and_then(|hex| {
            let bytes = hex::decode(hex).ok()?;
            bytes_to_versionstamp(&bytes)
        })
    }
}

/// Error from append_events.
#[derive(Debug)]
pub enum AppendError {
    ConditionFailed,
    Other(String),
}

impl std::fmt::Display for AppendError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AppendError::ConditionFailed => write!(f, "condition_failed"),
            AppendError::Other(s) => write!(f, "{s}"),
        }
    }
}

/// Append events with DCB conditions in a single FDB transaction.
pub async fn append_events(
    db: &Database,
    namespace: &str,
    events: Vec<EventToAppend>,
    conditions: Vec<AppendCondition>,
) -> Result<(), AppendError> {
    if events.is_empty() {
        return Err(AppendError::Other("events list is empty".into()));
    }

    let trx = db.create_trx().map_err(|e| AppendError::Other(format!("create_trx: {e}")))?;

    // Step 1: check all conditions (reads inside the same transaction)
    for condition in &conditions {
        if condition_exists(&trx, namespace, condition).await.map_err(AppendError::Other)? {
            return Err(AppendError::ConditionFailed);
        }
    }

    // Step 2: write all events
    for (batch_idx, event) in events.iter().enumerate() {
        let data = event.data_bytes().map_err(AppendError::Other)?;
        write_event(&trx, namespace, &event.event_type, &event.tags, &data, batch_idx as u16)
            .map_err(AppendError::Other)?;
    }

    // Step 3: commit
    trx.commit().await.map_err(|e| AppendError::Other(format!("commit: {e}")))?;
    Ok(())
}

async fn condition_exists(
    trx: &Transaction,
    namespace: &str,
    condition: &AppendCondition,
) -> Result<bool, String> {
    let after = condition.after_versionstamp();
    for item in &condition.query_items {
        if query_item_exists(trx, namespace, item, after.as_ref()).await? {
            return Ok(true);
        }
    }
    Ok(false)
}

async fn query_item_exists(
    trx: &Transaction,
    namespace: &str,
    item: &QueryItem,
    after: Option<&Versionstamp>,
) -> Result<bool, String> {
    let has_types = !item.types.is_empty();
    let has_tags = !item.tags.is_empty();

    if !has_types && !has_tags {
        return Err("condition query item must have at least one type or tag".into());
    }

    let ranges: Vec<(Vec<u8>, Vec<u8>)> = if has_types && !has_tags {
        item.types.iter().map(|t| {
            let p = type_index_prefix(namespace, t);
            if let Some(vs) = after { range_after_versionstamp(&p, vs) } else { prefix_range(&p) }
        }).collect()
    } else {
        let mut stags = item.tags.clone();
        stags.sort();
        let srefs: Vec<&str> = stags.iter().map(|s| s.as_str()).collect();
        let types = if has_types {
            item.types.clone()
        } else {
            discover_types_in_tag_subspace(trx, namespace, &srefs).await?
        };
        types.iter().map(|t| {
            let p = tag_index_type_prefix(namespace, &srefs, t);
            if let Some(vs) = after { range_after_versionstamp(&p, vs) } else { prefix_range(&p) }
        }).collect()
    };

    for (begin, end) in ranges {
        let range_opt = RangeOption {
            begin: KeySelector::first_greater_or_equal(begin),
            end: KeySelector::first_greater_or_equal(end),
            limit: Some(1),
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

fn write_event(
    trx: &Transaction,
    namespace: &str,
    event_type: &str,
    tags: &[String],
    data: &[u8],
    batch_index: u16,
) -> Result<(), String> {
    // Primary: {ns}/e/{vs} → tuple(type, tags, data)
    let (event_key, _) = event_key_incomplete(namespace, batch_index);
    let event_value = encode_event_value(event_type, tags, data);
    trx.atomic_op(&event_key, &event_value, MutationType::SetVersionstampedKey);

    // Type index: {ns}/t/{type}/{vs} → ""
    let (type_key, _) = type_index_key_incomplete(namespace, event_type, batch_index);
    trx.atomic_op(&type_key, &[], MutationType::SetVersionstampedKey);

    // Tag tree: for each non-empty subset of sorted tags
    for subset in generate_all_subsets(tags) {
        let refs: Vec<&str> = subset.iter().map(|s| s.as_str()).collect();
        let (tag_key, _) = tag_index_key_incomplete(namespace, &refs, event_type, batch_index);
        trx.atomic_op(&tag_key, &[], MutationType::SetVersionstampedKey);
    }

    Ok(())
}
