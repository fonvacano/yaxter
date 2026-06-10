package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, Error{Error: code, Message: message})
}

func unimplemented(w http.ResponseWriter) {
	writeError(w, http.StatusNotImplemented, "not_implemented",
		"this endpoint's task has not landed yet")
}

func formatID(id int64) string { return strconv.FormatInt(id, 10) }

func requireUser(w http.ResponseWriter, r *http.Request) (int64, bool) {
	uid, ok := UserID(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
	}
	return uid, ok
}
