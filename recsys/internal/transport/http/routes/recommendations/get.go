// Package recommendations — GET /api/v1/recommendations/me.
package recommendations

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"recsys/internal/auth"
	"recsys/internal/contract"
	"recsys/internal/domain"
	"recsys/internal/store/postgres"
	"recsys/internal/transport/http/respond"
)

// Handler умеет работать с реальной БД и с синтетическими примерами.
type Handler struct {
	Store       *postgres.Store
	ExamplesDir string
	Mock        bool
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized", "Missing or invalid session")
		return
	}

	if h.Mock {
		h.mock(w, r, profile)
		return
	}

	result, err := h.Store.Recommend(r.Context(), profile.ClientID, 3)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, result.RequestID, "store_error", err.Error())
		return
	}

	items := make([]domain.Recommendation, 0, len(result.Items))
	for _, item := range result.Items {
		url := item.URL
		items = append(items, domain.Recommendation{
			ItemID:      domain.ItemID(item.ItemID),
			OfferID:     &item.ItemID,
			Kind:        domain.Popular,
			Title:       item.Title,
			URL:         &url,
			Explanation: item.Reason,
			Variants:    []domain.Variant{},
			EvidenceIDs: []string{},
		})
	}

	respond.JSON(w, http.StatusOK, contract.RecommendationResponse{
		RequestID: result.RequestID,
		RecommendationResult: contract.RecommendationResult{
			SnapshotID: "live-catalog",
			Items:      items,
			Graph: domain.Graph{
				Nodes: []domain.Node{},
				Edges: []domain.Evidence{},
			},
			PartialReason: nil,
		},
	})
}

func (h *Handler) mock(w http.ResponseWriter, r *http.Request, profile auth.Profile) {
	scenario := r.URL.Query().Get("scenario")
	if scenario == "" {
		if profile.ID == "u2" {
			scenario = "partial"
		} else {
			scenario = "full"
		}
	}

	name := "recommendations." + scenario + ".json"
	path := filepath.Join(h.ExamplesDir, name)
	body, err := os.ReadFile(path)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "missing-example", "example_not_found", err.Error())
		return
	}

	status := http.StatusOK
	if scenario == "error" {
		status = http.StatusInternalServerError
	}
	if scenario == "slow" {
		select {
		case <-time.After(1500 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		body, err = os.ReadFile(filepath.Join(h.ExamplesDir, "recommendations.full.json"))
		if err != nil {
			respond.Error(w, http.StatusInternalServerError, "missing-example", "example_not_found", err.Error())
			return
		}
	}
	respond.Raw(w, status, body)
}

// NewRun создаёт новую выдачу в БД (не используется в mock-режиме).
func (h *Handler) NewRun(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized", "Missing or invalid session")
		return
	}
	if h.Mock {
		respond.JSON(w, http.StatusOK, map[string]any{"status": "mock", "profile_id": profile.ID})
		return
	}
	id, err := h.Store.ForceNewRun(r.Context(), profile.ClientID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "new-run", "store_error", err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"run_id": id, "profile_id": profile.ID})
}
