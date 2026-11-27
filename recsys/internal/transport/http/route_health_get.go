package transporthttp

import (
	"net/http"
	"time"

	"recsys/internal/transport/http/respond"
)

type healthResponseDTO struct {
	Status            string  `json:"status"`
	SchemaVersion     string  `json:"schema_version,omitempty"`
	CatalogVersion    *string `json:"catalog_version,omitempty"`
	FeedGeneratedAt   *string `json:"feed_generated_at,omitempty"`
	CatalogImportedAt *string `json:"catalog_imported_at,omitempty"`
}

func (s *Server) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	if s.health == nil {
		respond.JSON(w, r, http.StatusServiceUnavailable, healthResponseDTO{
			Status: "not_ready",
		})
		return
	}
	schemaVer, err := s.health.CheckHealth(r.Context())
	if err != nil {
		respond.JSON(w, r, http.StatusServiceUnavailable, healthResponseDTO{
			Status: "not_ready",
		})
		return
	}

	out := healthResponseDTO{
		Status:        "ok",
		SchemaVersion: schemaVer,
	}
	if s.catalogMeta != nil {
		if meta, err := s.catalogMeta.CatalogMeta(r.Context()); err == nil {
			out.CatalogVersion = meta.CatalogVersion
			if meta.FeedGeneratedAt != nil {
				v := meta.FeedGeneratedAt.UTC().Format(time.RFC3339)
				out.FeedGeneratedAt = &v
			}
			if meta.CatalogImportedAt != nil {
				v := meta.CatalogImportedAt.UTC().Format(time.RFC3339)
				out.CatalogImportedAt = &v
			}
		}
	}
	respond.JSON(w, r, http.StatusOK, out)
}
