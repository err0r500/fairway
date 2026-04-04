/// Read path: buildQueryRanges, type discovery, k-way merge.
use std::collections::{BinaryHeap, HashSet};
use std::cmp::Reverse;

use foundationdb::{Database, KeySelector, RangeOption, Transaction};
use foundationdb::tuple::Versionstamp;

use crate::keys::{
    type_index_prefix, tag_events_marker_prefix, tag_index_type_prefix,
    prefix_range, range_after_versionstamp,
    extract_versionstamp_from_key, extract_type_from_tag_key,
    decode_event_value, events_prefix, versionstamp_to_bytes, bytes_to_versionstamp,
};

/// A single query item: types (OR) + tags (AND).
#[derive(Debug, Clone, serde::Deserialize, serde::Serialize)]
pub struct QueryItem {
    #[serde(default)]
    pub types: Vec<String>,
    #[serde(default)]
    pub tags: Vec<String>,
}

/// Read options.
#[derive(Debug, Clone, Default, serde::Deserialize, serde::Serialize)]
pub struct ReadOpts {
    pub limit: Option<u32>,
    /// Hex-encoded 12-byte versionstamp.
    pub after_position: Option<String>,
    #[serde(default)]
    pub reverse: bool,
}

impl ReadOpts {
    pub fn after_versionstamp(&self) -> Option<Versionstamp> {
        self.after_position.as_deref().and_then(|hex| {
            let bytes = hex::decode(hex).ok()?;
            bytes_to_versionstamp(&bytes)
        })
    }
}

/// A decoded stored event.
#[derive(Debug, serde::Serialize, serde::Deserialize)]
pub struct StoredEvent {
    /// Hex-encoded 12-byte versionstamp.
    pub position: String,
    #[serde(rename = "type")]
    pub event_type: String,
    pub tags: Vec<String>,
    /// Base64-encoded event data bytes.
    pub data_b64: String,
}

impl StoredEvent {
    pub fn from_parts(vs: &Versionstamp, event_type: String, tags: Vec<String>, data: Vec<u8>) -> Self {
        Self {
            position: hex::encode(versionstamp_to_bytes(vs)),
            event_type,
            tags,
            data_b64: base64::Engine::encode(&base64::engine::general_purpose::STANDARD, &data),
        }
    }

    pub fn data_bytes(&self) -> Result<Vec<u8>, String> {
        base64::Engine::decode(&base64::engine::general_purpose::STANDARD, &self.data_b64)
            .map_err(|e| format!("base64 decode: {e}"))
    }
}

/// Read events matching the given query items within a single FDB ReadTransaction.
pub async fn read_events(
    db: &Database,
    namespace: &str,
    query_items: Vec<QueryItem>,
    opts: ReadOpts,
) -> Result<Vec<StoredEvent>, String> {
    let after = opts.after_versionstamp();
    let trx = db.create_trx().map_err(|e| format!("create_trx: {e}"))?;

    let mut all_ranges: Vec<(Vec<u8>, Vec<u8>)> = Vec::new();
    for item in &query_items {
        let ranges = build_query_ranges(&trx, namespace, item, after.as_ref()).await?;
        all_ranges.extend(ranges);
    }

    if all_ranges.is_empty() {
        return Ok(vec![]);
    }

    let mut per_range_keys: Vec<Vec<Vec<u8>>> = Vec::new();
    for (begin, end) in all_ranges {
        let range_opt = RangeOption {
            begin: KeySelector::first_greater_or_equal(begin),
            end: KeySelector::first_greater_or_equal(end),
            limit: None,
            reverse: opts.reverse,
            ..RangeOption::default()
        };
        let kvs = trx.get_range(&range_opt, 1, opts.reverse)
            .await
            .map_err(|e| format!("get_range: {e}"))?;
        let keys: Vec<Vec<u8>> = kvs.iter().map(|kv| kv.key().to_vec()).collect();
        per_range_keys.push(keys);
    }

    let ev_prefix = events_prefix(namespace);
    if opts.reverse {
        kway_merge_max(per_range_keys, &trx, &ev_prefix, opts.limit).await
    } else {
        kway_merge_min(per_range_keys, &trx, &ev_prefix, opts.limit).await
    }
}

/// Build FDB (begin, end) ranges for a single query item.
pub async fn build_query_ranges(
    trx: &Transaction,
    namespace: &str,
    item: &QueryItem,
    after: Option<&Versionstamp>,
) -> Result<Vec<(Vec<u8>, Vec<u8>)>, String> {
    let has_types = !item.types.is_empty();
    let has_tags = !item.tags.is_empty();

    if !has_types && !has_tags {
        return Err("query item must have at least one type or tag".into());
    }

    let mut ranges = Vec::new();

    if has_types && !has_tags {
        for t in &item.types {
            let prefix = type_index_prefix(namespace, t);
            ranges.push(if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            });
        }
    } else {
        let mut sorted_tags = item.tags.clone();
        sorted_tags.sort();
        let sorted_refs: Vec<&str> = sorted_tags.iter().map(|s| s.as_str()).collect();

        let types = if has_types {
            item.types.clone()
        } else {
            discover_types_in_tag_subspace(trx, namespace, &sorted_refs).await?
        };

        for t in &types {
            let prefix = tag_index_type_prefix(namespace, &sorted_refs, t);
            ranges.push(if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            });
        }
    }

    Ok(ranges)
}

/// Discover all event types stored under a tag set's _e subspace.
pub async fn discover_types_in_tag_subspace(
    trx: &Transaction,
    namespace: &str,
    sorted_tags: &[&str],
) -> Result<Vec<String>, String> {
    let marker_prefix = tag_events_marker_prefix(namespace, sorted_tags);
    let (begin, end) = prefix_range(&marker_prefix);

    let range_opt = RangeOption {
        begin: KeySelector::first_greater_or_equal(begin),
        end: KeySelector::first_greater_or_equal(end),
        ..RangeOption::default()
    };

    let kvs = trx.get_range(&range_opt, 1, false)
        .await
        .map_err(|e| format!("discover_types get_range: {e}"))?;

    let mut seen = HashSet::new();
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

/// Read all events from the primary events subspace in versionstamp order.
pub async fn read_all_events(db: &Database, namespace: &str) -> Result<Vec<StoredEvent>, String> {
    let trx = db.create_trx().map_err(|e| format!("create_trx: {e}"))?;
    let prefix = events_prefix(namespace);
    let (begin, end) = prefix_range(&prefix);

    let range_opt = RangeOption {
        begin: KeySelector::first_greater_or_equal(begin),
        end: KeySelector::first_greater_or_equal(end),
        ..RangeOption::default()
    };

    let kvs = trx.get_range(&range_opt, 1, false)
        .await
        .map_err(|e| format!("read_all get_range: {e}"))?;

    kvs.iter().map(|kv| {
        let vs = extract_versionstamp_from_key(kv.key())
            .ok_or_else(|| "failed to extract versionstamp".to_string())?;
        let (event_type, tags, data) = decode_event_value(kv.value())?;
        Ok(StoredEvent::from_parts(&vs, event_type, tags, data))
    }).collect()
}

// ── K-way merge ───────────────────────────────────────────────────────────────

async fn kway_merge_min(
    per_range_keys: Vec<Vec<Vec<u8>>>,
    trx: &Transaction,
    ev_prefix: &[u8],
    limit: Option<u32>,
) -> Result<Vec<StoredEvent>, String> {
    let mut heap: BinaryHeap<(Reverse<Vec<u8>>, usize, usize)> = BinaryHeap::new();
    for (ri, keys) in per_range_keys.iter().enumerate() {
        if !keys.is_empty() {
            let vs = extract_versionstamp_from_key(&keys[0]).ok_or("invalid key")?;
            heap.push((Reverse(versionstamp_to_bytes(&vs)), ri, 0));
        }
    }

    let mut result = Vec::new();
    let mut last_vs: Option<Vec<u8>> = None;

    while let Some((Reverse(vs_bytes), ri, ki)) = heap.pop() {
        if last_vs.as_deref() == Some(&vs_bytes) {
            let next_ki = ki + 1;
            if next_ki < per_range_keys[ri].len() {
                let vs = extract_versionstamp_from_key(&per_range_keys[ri][next_ki]).ok_or("invalid key")?;
                heap.push((Reverse(versionstamp_to_bytes(&vs)), ri, next_ki));
            }
            continue;
        }

        let vs = bytes_to_versionstamp(&vs_bytes).ok_or("invalid versionstamp")?;
        result.push(fetch_event(trx, ev_prefix, &vs).await?);
        last_vs = Some(vs_bytes);

        if limit.map_or(false, |l| result.len() >= l as usize) {
            break;
        }

        let next_ki = ki + 1;
        if next_ki < per_range_keys[ri].len() {
            let vs2 = extract_versionstamp_from_key(&per_range_keys[ri][next_ki]).ok_or("invalid key")?;
            heap.push((Reverse(versionstamp_to_bytes(&vs2)), ri, next_ki));
        }
    }

    Ok(result)
}

async fn kway_merge_max(
    per_range_keys: Vec<Vec<Vec<u8>>>,
    trx: &Transaction,
    ev_prefix: &[u8],
    limit: Option<u32>,
) -> Result<Vec<StoredEvent>, String> {
    let mut heap: BinaryHeap<(Vec<u8>, usize, usize)> = BinaryHeap::new();
    for (ri, keys) in per_range_keys.iter().enumerate() {
        if !keys.is_empty() {
            let vs = extract_versionstamp_from_key(&keys[0]).ok_or("invalid key")?;
            heap.push((versionstamp_to_bytes(&vs), ri, 0));
        }
    }

    let mut result = Vec::new();
    let mut last_vs: Option<Vec<u8>> = None;

    while let Some((vs_bytes, ri, ki)) = heap.pop() {
        if last_vs.as_deref() == Some(&vs_bytes) {
            let next_ki = ki + 1;
            if next_ki < per_range_keys[ri].len() {
                let vs = extract_versionstamp_from_key(&per_range_keys[ri][next_ki]).ok_or("invalid key")?;
                heap.push((versionstamp_to_bytes(&vs), ri, next_ki));
            }
            continue;
        }

        let vs = bytes_to_versionstamp(&vs_bytes).ok_or("invalid versionstamp")?;
        result.push(fetch_event(trx, ev_prefix, &vs).await?);
        last_vs = Some(vs_bytes);

        if limit.map_or(false, |l| result.len() >= l as usize) {
            break;
        }

        let next_ki = ki + 1;
        if next_ki < per_range_keys[ri].len() {
            let vs2 = extract_versionstamp_from_key(&per_range_keys[ri][next_ki]).ok_or("invalid key")?;
            heap.push((versionstamp_to_bytes(&vs2), ri, next_ki));
        }
    }

    Ok(result)
}

async fn fetch_event(
    trx: &Transaction,
    events_prefix: &[u8],
    vs: &Versionstamp,
) -> Result<StoredEvent, String> {
    // Append the encoded versionstamp element to the events prefix
    let mut vs_encoded = vec![0x33u8]; // FDB tuple versionstamp type code
    vs_encoded.extend_from_slice(vs.transaction_version());
    let uv = vs.user_version();
    vs_encoded.push((uv >> 8) as u8);
    vs_encoded.push(uv as u8);

    let mut event_key = events_prefix.to_vec();
    event_key.extend_from_slice(&vs_encoded);

    let value = trx.get(&event_key, false)
        .await
        .map_err(|e| format!("get event: {e}"))?
        .ok_or_else(|| format!("event not found for vs {}", hex::encode(versionstamp_to_bytes(vs))))?;

    let (event_type, tags, data) = decode_event_value(&value)?;
    Ok(StoredEvent::from_parts(vs, event_type, tags, data))
}
