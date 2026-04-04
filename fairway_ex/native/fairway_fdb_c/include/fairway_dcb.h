#ifndef FAIRWAY_DCB_H
#define FAIRWAY_DCB_H

#ifdef __cplusplus
extern "C" {
#endif

/**
 * Fairway DCB Store — C API
 *
 * Thread-safety: FwDb handles are safe to use from multiple goroutines/threads.
 * The underlying FDB client and Tokio runtime are both thread-safe.
 *
 * Ownership: all char* return values are heap-allocated and must be freed
 * exactly once with fw_free_string(). Never free them with free() directly.
 *
 * JSON data encoding:
 *   - Versionstamp positions: lowercase hex string, 24 characters (12 bytes).
 *   - Event data: base64-encoded (standard alphabet, no line breaks).
 *   - Tags: JSON array of strings.
 */

/**
 * Opaque handle to an open FDB database + namespace.
 * Created by fw_open_db(), destroyed by fw_close_db().
 */
typedef struct FwDbInner* FwDb;

/**
 * Open an FDB database.
 *
 * @param cluster_file_path  Path to the FDB cluster file, or NULL for the
 *                           default ($FDB_CLUSTER_FILE or
 *                           /etc/foundationdb/fdb.cluster).
 * @param namespace          Namespace prefix for all keys, or NULL for "fairway".
 *                           Use a unique namespace per application/environment.
 * @return  Opaque FwDb handle, or NULL if the database could not be opened.
 */
FwDb fw_open_db(const char* cluster_file_path, const char* namespace_);

/**
 * Close a database handle. Safe to call with NULL.
 */
void fw_close_db(FwDb db);

/**
 * Read events matching a query.
 *
 * @param db          Database handle from fw_open_db().
 * @param query_json  JSON array of query items:
 *                    [{"types":["EventType"],"tags":["tag:value"]}]
 *                    OR-semantics between items; types OR within item; tags AND within item.
 * @param opts_json   JSON object with optional fields:
 *                    {"limit":100,"after_position":"aabb...","reverse":false}
 *                    Pass "{}" or NULL for defaults.
 * @return  Heap-allocated JSON string. Free with fw_free_string().
 *          Success: {"events":[{"position":"hex24","type":"...","tags":[...],"data_b64":"..."},...]}
 *          Error:   {"error":"..."}
 */
char* fw_read_events(FwDb db, const char* query_json, const char* opts_json);

/**
 * Append events with optional DCB conditions.
 *
 * All events are written atomically in a single FDB transaction.
 * For each condition, the transaction checks: does any event matching
 * query_items exist after after_position? If yes, the transaction aborts
 * with condition_failed.
 *
 * @param db              Database handle.
 * @param events_json     JSON array of events to append:
 *                        [{"type":"EventType","tags":["tag:v"],"data_b64":"base64..."}]
 * @param conditions_json JSON array of conditions (may be "[]" for unconditional):
 *                        [{"query_items":[...],"after_position":"hex24"|null}]
 * @return  Heap-allocated JSON string. Free with fw_free_string().
 *          Success:           {"ok":true}
 *          Condition failure: {"error":"condition_failed"}
 *          Other error:       {"error":"..."}
 */
char* fw_append_events(FwDb db, const char* events_json, const char* conditions_json);

/**
 * Read all events in the namespace in versionstamp order.
 *
 * @param db  Database handle.
 * @return  Heap-allocated JSON string. Free with fw_free_string().
 *          Success: {"events":[...]}
 *          Error:   {"error":"..."}
 */
char* fw_read_all_events(FwDb db);

/**
 * Free a string returned by any fw_* function.
 * Safe to call with NULL.
 */
void fw_free_string(char* s);

#ifdef __cplusplus
}
#endif

#endif /* FAIRWAY_DCB_H */
