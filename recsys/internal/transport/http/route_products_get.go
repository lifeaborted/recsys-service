package transporthttp

import (
	"net/http"
	"strconv"

	"recsys/internal/transport/http/respond"
)

type productsResponseDTO struct {
	Items []ProductDTO `json:"items"`
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
	respond.JSON(w, r, http.StatusOK, productsResponseDTO{Items: items})
}
