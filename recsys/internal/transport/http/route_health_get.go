package transporthttp

import (
	"net/http"

	"recsys/internal/transport/http/respond"
)

func (s *Server) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	if s.health == nil {
		respond.JSON(w, r, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
		})
		return
	}
	schemaVer, err := s.health.CheckHealth(r.Context())
	if err != nil {
		respond.JSON(w, r, http.StatusServiceUnavailable, map[string]string{
			"status": "not_ready",
		})
		return
	}
	respond.JSON(w, r, http.StatusOK, map[string]string{
		"status":         "ok",
		"schema_version": schemaVer,
	})
}
