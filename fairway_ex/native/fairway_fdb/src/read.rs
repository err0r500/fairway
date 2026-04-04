/// Read path: buildQueryRanges, type discovery, k-way merge.
///
/// Mirrors Go's read.go and the buildQueryRanges/discoverTypesInTagSubspace
/// functions in dcb/dcb.go.
use std::collections::BinaryHeap;
use std::cmp::Reverse;

use foundationdb::{Database, RangeOption, KeySelector};
use foundationdb::tuple::Versionstamp;

use crate::keys::{
    type_index_prefix, tag_events_marker_prefix, tag_index_type_prefix,
    prefix_range, range_after_versionstamp,
    extract_versionstamp_from_key, extract_type_from_tag_key,
    decode_event_value, events_prefix,
};

/// A single query item: types (OR) + tags (AND).
#[derive(Debug, Clone)]
pub struct QueryItem {
    pub types: Vec<String>,
    pub tags: Vec<String>,
}

/// Read options.
#[derive(Debug, Clone, Default)]
pub struct ReadOpts {
    pub limit: Option<u32>,
    pub after_position: Option<Versionstamp>,
    pub reverse: bool,
}

/// A decoded stored event returned from read_events.
#[derive(Debug)]
pub struct StoredEvent {
    pub position: Versionstamp,
    pub event_type: String,
    pub tags: Vec<String>,
    pub data: Vec<u8>,
}

/// Read events matching the given query items within a single FDB ReadTransaction.
/// Returns all matching events, deduplicated and sorted by versionstamp.
pub async fn read_events(
    db: &Database,
    namespace: &str,
    query_items: Vec<QueryItem>,
    opts: ReadOpts,
) -> Result<Vec<StoredEvent>, String> {
    let namespace = namespace.to_string();
    let trx = db.create_trx().map_err(|e| format!("create_trx: {e}"))?;

    // Build all (begin, end) ranges from query items
    let mut all_ranges: Vec<(Vec<u8>, Vec<u8>)> = Vec::new();

    for item in &query_items {
        let ranges = build_query_ranges(&trx, &namespace, item, opts.after_position.as_ref()).await?;
        all_ranges.extend(ranges);
    }

    if all_ranges.is_empty() {
        return Ok(vec![]);
    }

    // Initialize iterators for all ranges
    // For each range, fetch all keys (we'll do k-way merge in memory)
    // FDB transactions are bounded (5s timeout, 10MB limit), so this is safe for normal use.
    let mut per_range_keys: Vec<Vec<Vec<u8>>> = Vec::new();
    for (begin, end) to all_ranges {
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

    // K-way merge with deduplication using a BinaryHeap
    // Each heap entry: (Reverse(position_bytes), range_index, key_index)
    // Reverse so that the smallest versionstamp is popped first (min-heap).
    // For reverse mode, we want largest first — use a max-heap (no Reverse).
    let events = if opts.reverse {
        kway_merge_max(per_range_keys, &trx, &namespace, opts.limit).await?
    } else {
        kway_merge_min(per_range_keys, &trx, &namespace, opts.limit).await?
    };

    Ok(events)
}

/// Build FDB (begin, end) ranges for a single query item.
async fn build_query_ranges(
    trx: &foundationdb::Transaction,
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
        // Type-only: one range per type in the type index
        for t in &item.types {
            let prefix = type_index_prefix(namespace, t);
            let range = if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            };
            ranges.push(range);
        }
    } else {
        // Tags (with or without types)
        let mut sorted_tags = item.tags.clone();
        sorted_tags.sort();
        let sorted_refs: Vec<&str> = sorted_tags.iter().map(|s| s.as_str()).collect();

        let types: Vec<String> = if has_types {
            item.types.clone()
        } else {
            // Tags-only: discover types from the _e subspace
            discover_types_in_tag_subspace(trx, namespace, &sorted_refs).await?
        };

        for t in &types {
            let prefix = tag_index_type_prefix(namespace, &sorted_refs, t);
            let range = if let Some(vs) = after {
                range_after_versionstamp(&prefix, vs)
            } else {
                prefix_range(&prefix)
            };
            ranges.push(range);
        }
    }

    Ok(ranges)
}

/// Discover all event types stored under a tag set's _e subspace.
/// Mirrors Go's discoverTypesInTagSubspace.
async fn discover_types_in_tag_subspace(
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

/// Read all events from the primary events subspace in versionstamp order.
pub async fn read_all_events(
    db: &Database,
    namespace: &str,
) -> Result<Vec<StoredEvent>, String> {
    let trx = db.create_trx().map_err(|e| format!("create_trx: {e}"))?;
    let prefix = events_prefix(namespace);
    let (begin, end) = prefix_range(&prefix);

    let range_opt = RangeOption {
        begin: KeySelector::first_greater_or_equal(begin),
        end: KeySelector::first_greater_or_equal(end),
        limit: None,
        reverse: false,
        ..RangeOption::default()
    };

    let kvs = trx.get_range(&range_opt, 1, false)
        .await
        .map_err(|e| format!("read_all get_range: {e}"))?;

    let mut events = Vec::new();
    for kv in kvs.iter() {
        let vs = extract_versionstamp_from_key(kv.key())
            .ok_or_else(|| "failed to extract versionstamp from event key".to_string())?;
        let (event_type, tags, data) = decode_event_value(kv.value())?;
        events.push(StoredEvent { position: vs, event_type, tags, data });
    }

    Ok(events)
}

// ── K-way merge helpers ───────────────────────────────────────────────────────

/// K-way merge producing events in ascending versionstamp order (min-heap).
async fn kway_merge_min(
    per_range_keys: Vec<Vec<Vec<u8>>>,
    trx: &foundationdb::Transaction,
    namespace: &str,
    limit: Option<u32>,
) -> Result<Vec<StoredEvent>, String> {
    // Heap entries: (Reverse(vs_bytes), range_idx, key_idx)
    let mut heap: BinaryHeap<(Reverse<Vec<u8>>, usize, usize)> = BinaryHeap::new();

    for (ri, keys) in per_range_keys.iter().enumerate() {
        if !keys.is_empty() {
            let vs = extract_versionstamp_from_key(&keys[0])
                .ok_or("invalid key in range")?;
            let vs_bytes = crate::keys::versionstamp_to_bytes(&vs);
            heap.push((Reverse(vs_bytes), ri, 0));
        }
    }

    let prefix = events_prefix(namespace);
    let mut result = Vec::new();
    let mut last_vs: Option<Vec<u8>> = None;

    while let Some((Reverse(vs_bytes), ri, ki)) = heap.pop() {
        // Deduplicate
        if last_vs.as_deref() == Some(&vs_bytes) {
            // Advance same iterator
            let next_ki = ki + 1;
            if next_ki < per_range_keys[ri].len() {
                let vs = extract_versionstamp_from_key(&per_range_keys[ri][next_ki])
                    .ok_or("invalid key")?;
                let vb = crate::keys::versionstamp_to_bytes(&vs);
                heap.push((Reverse(vb), ri, next_ki));
            }
            continue;
        }

        // Fetch event data from primary events subspace
        let vs = crate::keys::bytes_to_versionstamp(&vs_bytes)
            .ok_or("invalid versionstamp bytes")?;
        let event = fetch_event(trx, &prefix, &vs).await?;
        result.push(event);
        last_vs = Some(vs_bytes);

        if let Some(lim) = limit {
            if result.len() >= lim as usize {
                break;
            }
        }

        // Push next key from the same range
        let next_ki = ki + 1;
        if next_ki < per_range_keys[ri].len() {
            let vs2 = extract_versionstamp_from_key(&per_range_keys[ri][next_ki])
                .ok_or("invalid key")?;
            let vb2 = crate::keys::versionstamp_to_bytes(&vs2);
            heap.push((Reverse(vb2), ri, next_ki));
        }
    }

    Ok(result)
}

/// K-way merge producing events in descending versionstamp order (max-heap).
async fn kway_merge_max(
    per_range_keys: Vec<Vec<Vec<u8>>>,
    trx: &foundationdb::Transaction,
    namespace: &str,
    limit: Option<u32>,
) -> Result<Vec<StoredEvent>, String> {
    // Heap entries: (vs_bytes, range_idx, key_idx) — max-heap (largest first)
    let mut heap: BinaryHeap<(Vec<u8>, usize, usize)> = BinaryHeap::new();

    for (ri, keys) in per_range_keys.iter().enumerate() {
        if !keys.is_empty() {
            let vs = extract_versionstamp_from_key(&keys[0])
                .ok_or("invalid key in range")?;
            let vs_bytes = crate::keys::versionstamp_to_bytes(&vs);
            heap.push((vs_bytes, ri, 0));
        }
    }

    let prefix = events_prefix(namespace);
    let mut result = Vec::new();
    let mut last_vs: Option<Vec<u8>> = None;

    while let Some((vs_bytes, ri, ki)) = heap.pop() {
        if last_vs.as_deref() == Some(&vs_bytes) {
            let next_ki = ki + 1;
            if next_ki < per_range_keys[ri].len() {
                let vs = extract_versionstamp_from_key(&per_range_keys[ri][next_ki])
                    .ok_or("invalid key")?;
                let vb = crate::keys::versionstamp_to_bytes(&vs);
                heap.push((vb, ri, next_ki));
            }
            continue;
        }

        let vs = crate::keys::bytes_to_versionstamp(&vs_bytes)
            .ok_or("invalid versionstamp bytes")?;
        let event = fetch_event(trx, &prefix, &vs).await?;
        result.push(event);
        last_vs = Some(vs_bytes);

        if let Some(lim) = limit {
            if result.len() >= lim as usize {
                break;
            }
        }

        let next_ki = ki + 1;
        if next_ki < per_range_keys[ri].len() {
            let vs2 = extract_versionstamp_from_key(&per_range_keys[ri][next_ki])
                .ok_or("invalid key")?;
            let vb2 = crate::keys::versionstamp_to_bytes(&vs2);
            heap.push((vb2, ri, next_ki));
        }
    }

    Ok(result)
}

/// Fetch a single event from the primary events subspace by versionstamp.
async fn fetch_event(
    trx: &foundationdb::Transaction,
    events_prefix: &[u8],
    vs: &Versionstamp,
) -> Result<StoredEvent, String> {
    use crate::keys::versionstamp_to_bytes;
    use foundationdb::tuple::{pack, Element};

    // Reconstruct the event key: pack(events_prefix_elems ++ [vs])
    // events_prefix is already packed (namespace, "e"). We append the versionstamp.
    // We do this by packing the versionstamp element and appending to prefix bytes.
    // NOTE: This is a simplification — the correct approach is to use the namespace
    // and subspace to re-pack the complete key. Since our prefix IS the full subspace
    // prefix (packed tuple), we can append the vs element bytes directly.
    let vs_bytes = versionstamp_to_bytes(vs);

    // Build the event key: append encoded versionstamp element to prefix
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
        .ok_or_else(|| format!("event not found for versionstamp {:?}", vs_bytes))?;

    let (event_type, tags, data) = decode_event_value(&value)?;
    Ok(StoredEvent { position: *vs, event_type, tags, data })
}
