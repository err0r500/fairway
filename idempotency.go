package fairway

import (
	"net/http"

	"github.com/err0r500/fairway/dcb"
)

const idempotencyKeyHeader = "Idempotency-Key"

// idempotencyMiddleware wraps an http.HandlerFunc to provide idempotent request handling.
// If the request includes an Idempotency-Key header:
//   - Check the store for a cached response status code
//   - If found, return the cached status code without running the handler
//   - If not found, run the handler, capture the status code, and store it
//
// Requests without the header are passed through unchanged.
func idempotencyMiddleware(store dcb.IdempotencyStore, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(idempotencyKeyHeader)
		if key == "" {
			next(w, r)
			return
		}

		// Check if this key was already processed
		statusCode, found, err := store.Check(r.Context(), key)
		if err != nil {
			http.Error(w, "idempotency check failed", http.StatusInternalServerError)
			return
		}
		if found {
			w.WriteHeader(statusCode)
			return
		}

		// Run the handler with a response capture wrapper
		capture := &responseCapture{ResponseWriter: w}
		next(capture, r)

		// Store the result (best-effort; failure here doesn't affect the response)
		_ = store.Store(r.Context(), key, capture.statusCode)
	}
}

// responseCapture wraps http.ResponseWriter to capture the status code written by the handler.
type responseCapture struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rc *responseCapture) WriteHeader(code int) {
	if !rc.written {
		rc.statusCode = code
		rc.written = true
	}
	rc.ResponseWriter.WriteHeader(code)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	if !rc.written {
		rc.statusCode = http.StatusOK
		rc.written = true
	}
	return rc.ResponseWriter.Write(b)
}
