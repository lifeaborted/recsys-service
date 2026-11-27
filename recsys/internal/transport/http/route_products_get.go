package transporthttp

import (
	"net/http"
	"strconv"
	"time"

	"recsys/internal/auth"
	"recsys/internal/transport/http/respond"
)

type productsResponseDTO struct {
	CatalogVersion    *string      `json:"catalog_version,omitempty"`
	FeedGeneratedAt   *string      `json:"feed_generated_at,omitempty"`
	CatalogImportedAt *string      `json:"catalog_imported_at,omitempty"`
	Items             []ProductDTO `json:"items"`
	Total             int64        `json:"total"`
	Limit             int          `json:"limit"`
	Offset            int          `json:"offset"`
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
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			respond.Error(w, r, http.StatusBadRequest, "invalid_offset", "offset must be a non-negative integer")
			return
		}
		offset = parsed
	}
	query := r.URL.Query().Get("q")
	withHistory := r.URL.Query().Get("with_history") == "1" || r.URL.Query().Get("with_history") == "true"

	profile, _ := auth.Current(r.Context())
	if withHistory && profile.Role != "admin" {
		respond.Error(w, r, http.StatusForbidden, "admin_required", "Administrator access required")
		return
	}

	var items []ProductDTO
	var total int64
	var err error
	if paged, ok := s.catalog.(ProductPageStore); ok {
		items, total, err = paged.ListProductsPage(r.Context(), limit, offset, query, withHistory)
	} else {
		items, err = s.catalog.ListProducts(r.Context(), limit)
		total = int64(len(items))
	}
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "store_error", "Failed to load catalog products")
		return
	}
	if items == nil {
		items = []ProductDTO{}
	}

	if profile.Role != "admin" {
		for i := range items {
			items[i].HasHistory = false
		}
	}

	out := productsResponseDTO{Items: items, Total: total, Limit: limit, Offset: offset}
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
