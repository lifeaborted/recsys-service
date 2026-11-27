package transporthttp

import (
	"net/http"
	"time"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/transport/http/respond"
)

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, _ := auth.Current(r.Context())
		if profile.Role != "admin" {
			respond.Error(w, r, http.StatusForbidden, "admin_required", "Administrator access required")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next(w, r)
	}
}

func (s *Server) handleGetAdminRecommendations(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		target = "guest"
	}
	clientID := map[string]domain.ClientID{
		"guest": "DEMO-GUEST", "notver": "DEMO-NOTVER",
		"verb2b": "DEMO-VERB2B", "verb2c": "DEMO-VERB2C", "admin": "DEMO-ADMIN",
	}[target]
	if clientID == "" {
		respond.Error(w, r, http.StatusBadRequest, "invalid_target", "Unknown preview profile")
		return
	}

	start := time.Now()
	result, ok := s.recommendationsFor(w, r, clientID)
	if !ok {
		return
	}
	elapsed := time.Since(start).Milliseconds()
	runs := []contract.RunRecord{}
	var tables map[string]int64
	if s.stats != nil {
		var err error
		tables, err = s.stats.TableStats(r.Context())
		if err != nil {
			respond.Error(w, r, http.StatusInternalServerError, "stats_failed", "Failed to read table statistics")
			return
		}
	}
	if s.runs != nil && target != "guest" {
		var err error
		runs, err = s.runs.ListRuns(r.Context(), clientID, 20)
		if err != nil {
			respond.Error(w, r, http.StatusInternalServerError, "runs_failed", "Failed to read stored runs")
			return
		}
	}
	respond.JSON(w, r, http.StatusOK, struct {
		Target         string                          `json:"target"`
		DurationMillis int64                           `json:"duration_ms"`
		Result         contract.RecommendationResponse `json:"recommendations"`
		Tables         map[string]int64                `json:"tables"`
		Runs           []contract.RunRecord            `json:"runs"`
	}{target, elapsed, result, tables, runs})
}
