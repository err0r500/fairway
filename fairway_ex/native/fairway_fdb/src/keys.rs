/// Key encoding/decoding for the Fairway DCB store.
///
/// Subspace layout (mirrors Go dcb package):
///   {namespace}/e/{versionstamp}                       → tuple(type, tags, data)
///   {namespace}/t/{type}/{versionstamp}                → b""
///   {namespace}/g/{tag1}/{tag2}/.../_e/{type}/{vs}     → b""
///
/// Versionstamp: 12 bytes = 10-byte FDB transaction version + 2-byte user version (big-endian).
/// FDB tuple encoding is used throughout (compatible with the Go fdb/tuple package).
use foundationdb::tuple::{pack, unpack, Element, Versionstamp};

pub const EVENTS_SUBSPACE: &str = "e";
pub const TYPE_INDEX_SUBSPACE: &str = "t";
pub const TAG_INDEX_SUBSPACE: &str = "g";
pub const TAG_EVENTS_MARKER: &str = "_e";

// ── Versionstamp helpers ─────────────────────────────────────────────────────

/// Convert raw 12-byte slice to FDB Versionstamp (10-byte tx_version + 2-byte user_version).
pub fn bytes_to_versionstamp(b: &[u8]) -> Option<Versionstamp> {
    if b.len() != 12 {
        return None;
    }
    let mut tx_version = [0u8; 10];
    tx_version.copy_from_slice(&b[..10]);
    let user_version = u16::from_be_bytes([b[10], b[11]]);
    Some(Versionstamp::complete(tx_version, user_version))
}

/// Convert FDB Versionstamp to raw 12-byte Vec.
pub fn versionstamp_to_bytes(vs: &Versionstamp) -> Vec<u8> {
    let mut out = Vec::with_capacity(12);
    out.extend_from_slice(vs.transaction_version());
    let uv = vs.user_version();
    out.push((uv >> 8) as u8);
    out.push((uv & 0xFF) as u8);
    out
}

// ── Subspace prefix helpers ───────────────────────────────────────────────────

/// Pack a namespace prefix: tuple(namespace).
pub fn namespace_prefix(namespace: &str) -> Vec<u8> {
    pack(&(namespace,))
}

/// Pack the events subspace prefix: tuple(namespace, "e").
pub fn events_prefix(namespace: &str) -> Vec<u8> {
    pack(&(namespace, EVENTS_SUBSPACE))
}

/// Pack a primary event key with an incomplete versionstamp at the given batch index.
/// FDB will fill in the versionstamp on commit.
/// Returns (key_bytes, versionstamp_offset).
pub fn event_key_incomplete(namespace: &str, batch_index: u16) -> (Vec<u8>, u32) {
    let vs = Versionstamp::incomplete(batch_index);
    let tuple: Vec<Element> = vec![
        Element::String(namespace.into()),
        Element::String(EVENTS_SUBSPACE.into()),
        Element::Versionstamp(vs),
    ];
    let (packed, offset) = pack_with_versionstamp_offset(&tuple);
    (packed, offset)
}

/// Pack the type index prefix: tuple(namespace, "t", type).
pub fn type_index_prefix(namespace: &str, event_type: &str) -> Vec<u8> {
    pack(&(namespace, TYPE_INDEX_SUBSPACE, event_type))
}

/// Pack a type index key with an incomplete versionstamp.
pub fn type_index_key_incomplete(namespace: &str, event_type: &str, batch_index: u16) -> (Vec<u8>, u32) {
    let vs = Versionstamp::incomplete(batch_index);
    let tuple: Vec<Element> = vec![
        Element::String(namespace.into()),
        Element::String(TYPE_INDEX_SUBSPACE.into()),
        Element::String(event_type.into()),
        Element::Versionstamp(vs),
    ];
    let (packed, offset) = pack_with_versionstamp_offset(&tuple);
    (packed, offset)
}

/// Pack a tag tree index key prefix up to the type level:
/// tuple(namespace, "g", tag1, tag2, ..., "_e", type).
pub fn tag_index_type_prefix(namespace: &str, sorted_tags: &[&str], event_type: &str) -> Vec<u8> {
    let mut elems: Vec<Element> = Vec::with_capacity(sorted_tags.len() + 4);
    elems.push(Element::String(namespace.into()));
    elems.push(Element::String(TAG_INDEX_SUBSPACE.into()));
    for tag in sorted_tags {
        elems.push(Element::String((*tag).into()));
    }
    elems.push(Element::String(TAG_EVENTS_MARKER.into()));
    elems.push(Element::String(event_type.into()));
    pack_elements(&elems)
}

/// Pack a tag tree index key prefix up to the _e marker (no type):
/// tuple(namespace, "g", tag1, tag2, ..., "_e").
pub fn tag_events_marker_prefix(namespace: &str, sorted_tags: &[&str]) -> Vec<u8> {
    let mut elems: Vec<Element> = Vec::with_capacity(sorted_tags.len() + 3);
    elems.push(Element::String(namespace.into()));
    elems.push(Element::String(TAG_INDEX_SUBSPACE.into()));
    for tag in sorted_tags {
        elems.push(Element::String((*tag).into()));
    }
    elems.push(Element::String(TAG_EVENTS_MARKER.into()));
    pack_elements(&elems)
}

/// Pack a tag tree index key with an incomplete versionstamp.
pub fn tag_index_key_incomplete(
    namespace: &str,
    sorted_tags: &[&str],
    event_type: &str,
    batch_index: u16,
) -> (Vec<u8>, u32) {
    let vs = Versionstamp::incomplete(batch_index);
    let mut elems: Vec<Element> = Vec::with_capacity(sorted_tags.len() + 5);
    elems.push(Element::String(namespace.into()));
    elems.push(Element::String(TAG_INDEX_SUBSPACE.into()));
    for tag in sorted_tags {
        elems.push(Element::String((*tag).into()));
    }
    elems.push(Element::String(TAG_EVENTS_MARKER.into()));
    elems.push(Element::String(event_type.into()));
    elems.push(Element::Versionstamp(vs));
    let (packed, offset) = pack_with_versionstamp_offset(&elems);
    (packed, offset)
}

// ── Event value encoding/decoding ─────────────────────────────────────────────

/// Encode an event value as a tuple: (type, (tag1, tag2, ...), data_bytes).
pub fn encode_event_value(event_type: &str, tags: &[String], data: &[u8]) -> Vec<u8> {
    let tags_tuple: Vec<Element> = tags.iter().map(|t| Element::String(t.clone().into())).collect();
    let elems: Vec<Element> = vec![
        Element::String(event_type.into()),
        Element::Tuple(tags_tuple),
        Element::Bytes(data.into()),
    ];
    pack_elements(&elems)
}

/// Decode an event value from tuple bytes.
/// Returns (type, tags, data_bytes).
pub fn decode_event_value(value: &[u8]) -> Result<(String, Vec<String>, Vec<u8>), String> {
    let elems: Vec<Element> = unpack(value).map_err(|e| format!("tuple unpack error: {e}"))?;
    if elems.len() != 3 {
        return Err(format!("expected 3-element tuple, got {}", elems.len()));
    }

    let event_type = match &elems[0] {
        Element::String(s) => s.to_string(),
        other => return Err(format!("type field: expected String, got {:?}", other)),
    };

    let tags = match &elems[1] {
        Element::Tuple(t) => t
            .iter()
            .map(|e| match e {
                Element::String(s) => Ok(s.to_string()),
                other => Err(format!("tag element: expected String, got {:?}", other)),
            })
            .collect::<Result<Vec<_>, _>>()?,
        other => return Err(format!("tags field: expected Tuple, got {:?}", other)),
    };

    let data = match &elems[2] {
        Element::Bytes(b) => b.to_vec(),
        other => return Err(format!("data field: expected Bytes, got {:?}", other)),
    };

    Ok((event_type, tags, data))
}

/// Extract the versionstamp from an index key by unpacking the last tuple element.
/// All index keys end with a versionstamp element.
pub fn extract_versionstamp_from_key(key: &[u8]) -> Option<Versionstamp> {
    let elems: Vec<Element> = unpack(key).ok()?;
    match elems.last()? {
        Element::Versionstamp(vs) => Some(*vs),
        _ => None,
    }
}

/// Extract the event type from a tag index key.
/// Tag index keys are: (namespace, "g", tag1, ..., "_e", type, versionstamp)
/// The type is the second-to-last element.
pub fn extract_type_from_tag_key(key: &[u8]) -> Option<String> {
    let elems: Vec<Element> = unpack(key).ok()?;
    if elems.len() < 2 {
        return None;
    }
    match &elems[elems.len() - 2] {
        Element::String(s) => Some(s.to_string()),
        _ => None,
    }
}

/// Build the FDB range for a given prefix (all keys with that prefix).
pub fn prefix_range(prefix: &[u8]) -> (Vec<u8>, Vec<u8>) {
    let begin = prefix.to_vec();
    let mut end = prefix.to_vec();
    // Increment last byte, or append \xff if at boundary
    strinc(&mut end);
    (begin, end)
}

/// Build the FDB range starting strictly after a given versionstamp within a prefix.
/// begin = pack(prefix_elems ++ [versionstamp]) ++ b"\x00"
/// end   = end of prefix range
pub fn range_after_versionstamp(type_prefix: &[u8], after: &Versionstamp) -> (Vec<u8>, Vec<u8>) {
    let vs_bytes = versionstamp_to_bytes(after);
    // Pack just the versionstamp element to get its tuple encoding, then append
    // to the prefix to form the "at" key, then add \x00 to make it exclusive.
    let vs_elem_packed = pack(&(Element::Versionstamp(*after),));
    // The prefix ends with the subspace bytes; the vs element follows.
    // begin_at = type_prefix + vs_packed_bytes (minus leading 0x00 of empty tuple prefix)
    // Actually: we need type_prefix + vs_tuple_bytes
    // type_prefix is already a packed tuple. Appending the versionstamp element
    // means we need to re-pack the whole key. We do it differently:
    // use the fact that FDB range "after VS" = key(VS) + 0x00 as begin.
    let _ = vs_bytes; // not used directly

    // Reconstruct: strip the trailing \xff\xff from the prefix range end,
    // use type_prefix + pack([vs]) as the "at" key.
    // Simpler: build begin = type_prefix ++ encoded_vs ++ 0x00
    // The vs element in FDB tuple encoding is 0x33 + 12 bytes.
    let mut vs_encoded = vec![0x33u8]; // FDB tuple versionstamp type code
    vs_encoded.extend_from_slice(after.transaction_version());
    let uv = after.user_version();
    vs_encoded.push((uv >> 8) as u8);
    vs_encoded.push((uv & 0xFF) as u8);

    let mut begin = type_prefix.to_vec();
    begin.extend_from_slice(&vs_encoded);
    begin.push(0x00); // exclusive: start AFTER this key

    let (_, end) = prefix_range(type_prefix);
    (begin, end)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

fn pack_elements(elems: &[Element]) -> Vec<u8> {
    // We need to pack a slice of Element values.
    // foundationdb's pack() works on tuples implementing TuplePack.
    // For a Vec<Element>, we can iterate and pack individually then combine,
    // or use a workaround.
    // Use tuple::pack on a slice reference.
    foundationdb::tuple::pack(elems)
}

/// Pack a tuple containing a versionstamp, returning (packed_bytes, versionstamp_offset).
/// The offset points to where FDB should write the 10-byte transaction version.
fn pack_with_versionstamp_offset(elems: &[Element]) -> (Vec<u8>, u32) {
    foundationdb::tuple::pack_into_with_versionstamp(elems)
}

/// FDB strinc: increment the last byte of a key for range end.
fn strinc(key: &mut Vec<u8>) {
    for i in (0..key.len()).rev() {
        if key[i] < 0xFF {
            key[i] += 1;
            return;
        }
        key.pop();
    }
    // All bytes were 0xFF — the result is empty, meaning "end of keyspace"
}
