package transporthttp

import (
	"net/http"

	"recsys/internal/transport/http/respond"
)

type statsResponseDTO struct {
	Status       string           `json:"status"`
	ModelVersion string           `json:"model_version"`
	Tables       map[string]int64 `json:"tables"`
}

func (s *Server) handleGetStats(w http.ResponseWriter, r *http.Request) {
	if s.stats == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "stats_unavailable", "Statistics store unavailable")
		return
	}
	tables, err := s.stats.TableStats(r.Context())
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "stats_failed", "Failed to read table statistics")
		return
	}
	version := ""
	if s.health != nil {
		version, _ = s.health.CheckHealth(r.Context())
	}
	respond.JSON(w, r, http.StatusOK, statsResponseDTO{
		Status:       "ok",
		ModelVersion: version,
		Tables:       tables,
	})
}
