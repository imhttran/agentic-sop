package web

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON writes v as a JSON response (activity endpoints).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; all we can do is log.
		log.Printf("[json] encode: %v", err)
	}
}
