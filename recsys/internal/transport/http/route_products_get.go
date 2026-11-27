package transporthttp

import (
	"net/http"
	"strconv"
	"time"

	"recsys/internal/transport/http/respond"
)

type productsResponseDTO struct {
	CatalogVersion    *string      `json:"catalog_version,omitempty"`
	FeedGeneratedAt   *string      `json:"feed_generated_at,omitempty"`
	CatalogImportedAt *string      `json:"catalog_imported_at,omitempty"`
	Items             []ProductDTO `json:"items"`
}

func (s *Server) handleGetProducts(w http.ResponseWriter, r *http.Request) {
	if s.catalog == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "Catalog store unavailable")
		return
	}

	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			respond.Error(w, r, http.StatusBadRequest, "invalid_limit", "limit must be an integer between 1 and 100")
			return
		}
		limit = parsed
	}

	items, err := s.catalog.ListProducts(r.Context(), limit)
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "store_error", "Failed to load catalog products")
		return
	}
	if items == nil {
		items = []ProductDTO{}
	}

	out := productsResponseDTO{Items: items}
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
