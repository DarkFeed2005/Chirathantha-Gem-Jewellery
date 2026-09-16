// Package httputil holds small helpers shared by every handler so response
// shapes stay consistent across the API instead of each handler rolling
// its own JSON encoding.
package httputil

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type errorBody struct {
	Error string `json:"error"`
}

// WriteJSON encodes v as JSON with the given status code. Encoding errors
// are logged, not panicked on — by the time we're writing the body,
// there's nothing useful left to do but note it happened.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httputil: failed to encode JSON response", "error", err)
	}
}

// WriteError writes a consistent {"error": "..."} body. The message
// passed here is always safe to show a client — never pass a raw
// internal/database error string to this function; log the real error
// server-side and pass a clean message instead.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, errorBody{Error: message})
}
