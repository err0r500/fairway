package utils

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"time"

	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/apple/foundationdb/bindings/go/src/fdb/subspace"
	"github.com/apple/foundationdb/bindings/go/src/fdb/tuple"
	"github.com/go-playground/validator/v10"
)

const (
	idempotencyHeader         = "Idempotency-Key"
	idempotencyDefaultTimeout = 10 * time.Second
	idempotencyPollInterval   = 50 * time.Millisecond

	// Marker value stored while the request is being processed.
	// Once complete, the value is replaced with the actual response.
	idempotencyProcessingMarker = "__processing__"
)

// JsonParse decodes JSON and validates struct
// Returns error for caller to handle (decode or validation errors)
func JsonParse[T any](r *http.Request, v *T) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return err
	}
	if err := validator.New().Struct(v); err != nil {
		return err
	}
	return nil
}

// IdempotencyMiddleware returns an http.Handler that deduplicates requests
// sharing the same Idempotency-Key header. The first request with a given key
// executes next; concurrent duplicates wait (up to 10s) for that result.
// Responses (status code + body) are stored in a dedicated FDB subspace.
func IdempotencyMiddleware(db fdb.Database, namespace string, next http.Handler) http.Handler {
	ss := subspace.Sub(namespace).Sub("idempotency")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(idempotencyHeader)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		fdbKey := ss.Pack(tuple.Tuple{key})

		// Try to claim the key atomically.
		claimed, existingValue, err := tryClaim(db, fdbKey)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if claimed {
			// We own this key — execute the real handler.
			rec := &responseRecorder{header: make(http.Header), body: &bytes.Buffer{}, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			encoded := encodeResponse(rec.statusCode, rec.body.Bytes())
			if err := storeResult(db, fdbKey, encoded); err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			writeRecordedResponse(w, rec.statusCode, rec.body.Bytes())
			return
		}

		// Another request is processing this key. If the value is already
		// the final response (not the processing marker), return it directly.
		if !isProcessing(existingValue) {
			code, body := decodeResponse(existingValue)
			writeRecordedResponse(w, code, body)
			return
		}

		// Wait for the result using polling + timeout.
		result, err := waitForResult(db, fdbKey, idempotencyDefaultTimeout)
		if err != nil {
			http.Error(w, "idempotency timeout", http.StatusGatewayTimeout)
			return
		}

		code, body := decodeResponse(result)
		writeRecordedResponse(w, code, body)
	})
}

// tryClaim attempts to atomically set the processing marker on the key.
// Returns (true, nil, nil) if claimed, (false, existingValue, nil) if already taken.
func tryClaim(db fdb.Database, fdbKey fdb.Key) (bool, []byte, error) {
	type claimResult struct {
		claimed  bool
		existing []byte
	}
	res, err := db.Transact(func(tr fdb.Transaction) (any, error) {
		val := tr.Get(fdbKey).MustGet()
		if val != nil {
			return claimResult{claimed: false, existing: val}, nil
		}
		tr.Set(fdbKey, []byte(idempotencyProcessingMarker))
		return claimResult{claimed: true}, nil
	})
	if err != nil {
		return false, nil, err
	}
	cr := res.(claimResult)
	return cr.claimed, cr.existing, nil
}

// storeResult overwrites the processing marker with the encoded response.
func storeResult(db fdb.Database, fdbKey fdb.Key, encoded []byte) error {
	_, err := db.Transact(func(tr fdb.Transaction) (any, error) {
		tr.Set(fdbKey, encoded)
		return nil, nil
	})
	return err
}

// waitForResult polls FDB until the value is no longer the processing marker
// or until the timeout expires.
func waitForResult(db fdb.Database, fdbKey fdb.Key, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)

	for {
		res, err := db.ReadTransact(func(tr fdb.ReadTransaction) (any, error) {
			futureVal := tr.Get(fdbKey)
			val := futureVal.MustGet()
			return val, nil
		})
		if err != nil {
			return nil, err
		}

		val := res.([]byte)
		if val != nil && !isProcessing(val) {
			return val, nil
		}

		if time.Now().After(deadline) {
			return nil, http.ErrHandlerTimeout
		}

		// Brief sleep before next poll.
		remaining := time.Until(deadline)
		sleep := idempotencyPollInterval
		if sleep > remaining {
			sleep = remaining
		}
		time.Sleep(sleep)
	}
}

func isProcessing(val []byte) bool {
	return bytes.Equal(val, []byte(idempotencyProcessingMarker))
}

// encodeResponse packs a status code (4 bytes big-endian) followed by the body.
func encodeResponse(statusCode int, body []byte) []byte {
	buf := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(buf[:4], uint32(statusCode))
	copy(buf[4:], body)
	return buf
}

// decodeResponse unpacks status code and body from the encoded format.
func decodeResponse(data []byte) (int, []byte) {
	if len(data) < 4 {
		return http.StatusInternalServerError, nil
	}
	code := int(binary.BigEndian.Uint32(data[:4]))
	body := data[4:]
	return code, body
}

func writeRecordedResponse(w http.ResponseWriter, statusCode int, body []byte) {
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
}

// responseRecorder captures the response from the next handler.
type responseRecorder struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) Header() http.Header        { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *responseRecorder) WriteHeader(statusCode int)  { r.statusCode = statusCode }
