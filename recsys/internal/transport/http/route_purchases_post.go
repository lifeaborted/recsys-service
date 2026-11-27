package transporthttp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/transport/http/respond"
)

type createPurchaseRequestDTO struct {
	ItemID string `json:"item_id"`
}

func (s *Server) handlePostPurchases(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	if s.purchases == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "purchases_unavailable", "Purchases store unavailable")
		return
	}

	var req createPurchaseRequestDTO
	if err := decodeStrictJSON(r, &req); err != nil {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Invalid purchase payload")
		return
	}
	itemID := strings.TrimSpace(req.ItemID)
	if itemID == "" {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "item_id is required")
		return
	}

	recorded, err := s.purchases.CreatePurchase(r.Context(), profile.ClientID, itemID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrUnknownItem) {
			respond.Error(w, r, http.StatusNotFound, "unknown_item", "Unknown item")
			return
		}
		respond.Error(w, r, http.StatusInternalServerError, "store_error", "Failed to record purchase")
		return
	}

	respond.JSON(w, r, http.StatusCreated, recorded)
}

func (s *Server) handleGetPurchases(w http.ResponseWriter, r *http.Request) {
	profile, _ := auth.Current(r.Context())
	store, ok := s.purchases.(interface {
		ListPurchases(context.Context, domain.ClientID) ([]contract.PurchaseHistoryItem, error)
	})
	if !ok {
		respond.Error(w, r, http.StatusServiceUnavailable, "purchases_unavailable", "Purchases store unavailable")
		return
	}
	items, err := store.ListPurchases(r.Context(), profile.ClientID)
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "store_error", "Failed to load purchases")
		return
	}
	respond.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}
