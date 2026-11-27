package transporthttp

import (
	"errors"
	"net/http"

	"recsys/internal/contract"
	"recsys/internal/transport/http/respond"
)

func (s *Server) handleGetProductDetail(w http.ResponseWriter, r *http.Request) {
	if s.productDetails == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "Product detail store unavailable")
		return
	}
	itemID := r.PathValue("itemID")
	if itemID == "" {
		respond.Error(w, r, http.StatusBadRequest, "invalid_item", "item_id is required")
		return
	}
	detail, err := s.productDetails.ProductDetail(r.Context(), itemID)
	if err != nil {
		if errors.Is(err, contract.ErrProductNotFound) {
			respond.Error(w, r, http.StatusNotFound, "product_not_found", "Product not found")
			return
		}
		respond.Error(w, r, http.StatusInternalServerError, "product_failed", "Failed to read product detail")
		return
	}
	respond.JSON(w, r, http.StatusOK, detail)
}
