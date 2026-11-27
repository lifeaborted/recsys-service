package transporthttp

import (
	"net/http"

	"recsys/internal/transport/http/respond"
)

func (s *Server) handleGetScoreHistory(w http.ResponseWriter, r *http.Request) {
	if s.scoreHistory == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "history_unavailable", "Score history store unavailable")
		return
	}
	itemID := r.PathValue("itemID")
	if itemID == "" {
		respond.Error(w, r, http.StatusBadRequest, "invalid_item", "item_id is required")
		return
	}
	history, err := s.scoreHistory.ScoreHistory(r.Context(), itemID)
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "history_failed", "Failed to read score history")
		return
	}
	respond.JSON(w, r, http.StatusOK, history)
}
