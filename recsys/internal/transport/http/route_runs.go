package transporthttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/transport/http/respond"
)

type createRunRequest struct {
	ModelVersion string                  `json:"model_version,omitempty"`
	Items        []contract.RunItemInput `json:"items"`
}

type runsListResponse struct {
	UserID   string               `json:"user_id"`
	ClientID string               `json:"client_id"`
	Runs     []contract.RunRecord `json:"runs"`
}

type createEventRequest struct {
	RunID     int64  `json:"run_id"`
	OfferID   string `json:"offer_id"`
	EventType string `json:"event_type"`
}

func (s *Server) handlePostRuns(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "runs_unavailable", "Run store unavailable")
		return
	}
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	var body createRunRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Invalid JSON body")
		return
	}
	if len(body.Items) == 0 || len(body.Items) > 3 {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "items must contain 1 to 3 entries")
		return
	}
	run, err := s.runs.CreateRun(r.Context(), profile.ClientID, body.ModelVersion, body.Items)
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "run_failed", "Failed to create run")
		return
	}
	respond.JSON(w, r, http.StatusCreated, run)
}

func (s *Server) handleGetRuns(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "runs_unavailable", "Run store unavailable")
		return
	}
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			respond.Error(w, r, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
	}
	runs, err := s.runs.ListRuns(r.Context(), profile.ClientID, limit)
	if err != nil {
		respond.Error(w, r, http.StatusInternalServerError, "runs_failed", "Failed to read runs")
		return
	}
	respond.JSON(w, r, http.StatusOK, runsListResponse{
		UserID:   profile.ID,
		ClientID: string(profile.ClientID),
		Runs:     runs,
	})
}

func (s *Server) handlePostEvents(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "runs_unavailable", "Run store unavailable")
		return
	}
	var body createEventRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Invalid JSON body")
		return
	}
	if body.RunID <= 0 || body.OfferID == "" || (body.EventType != "click" && body.EventType != "purchase") {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "run_id, offer_id and click/purchase are required")
		return
	}
	profile, _ := auth.Current(r.Context())
	if err := s.runs.CreateEvent(r.Context(), profile.ClientID, body.RunID, body.OfferID, body.EventType); err != nil {
		if errors.Is(err, contract.ErrRunNotFound) {
			respond.Error(w, r, http.StatusNotFound, "run_not_found", "Run or offer not found")
			return
		}
		respond.Error(w, r, http.StatusInternalServerError, "event_failed", "Failed to record event")
		return
	}
	respond.JSON(w, r, http.StatusCreated, map[string]any{"accepted": true})
}
