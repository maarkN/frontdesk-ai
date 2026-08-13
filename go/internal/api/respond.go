package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/maarkn/frontdesk/internal/domain"
)

// errorBody is the uniform error response shape.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON encodes v with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the uniform error body.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

// writeStoreError maps domain sentinel errors to HTTP statuses.
func writeStoreError(w http.ResponseWriter, err error, notFoundMsg string) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, notFoundMsg)
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

// decodeJSON strictly decodes the request body into dst.
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
