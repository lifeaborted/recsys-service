package health

import (
	"encoding/json"
	"net/http"
)

// Get reports readiness. The scaffold has no database or schema check wired in.
func Get(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{Status: "not_ready"})
}
