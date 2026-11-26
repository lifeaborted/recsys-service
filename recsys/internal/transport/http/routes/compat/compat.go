// Package compat — эндпоинты старого клиента ЛК (/api/...).
// Позволяют странице «Тест рекомендаций» работать через recsys API
// без отдельного сервиса на :8000.
package compat

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"recsys/internal/auth"
	"recsys/internal/store/postgres"
	"recsys/internal/transport/http/respond"
)

type Handler struct {
	Store        *postgres.Store
	ExamplesDir  string
	Mock         bool
	ModelVersion string
}

func (h *Handler) profileFor(userID string) auth.Profile {
	profile, ok := auth.FromToken("demo-token-" + userID)
	if !ok {
		profile, _ = auth.FromToken("demo-token-u1")
	}
	return profile
}

func (h *Handler) Recommendations(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		respond.Error(w, http.StatusBadRequest, "bad-request", "invalid_input", "user_id is required")
		return
	}
	limit := 9
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 50 {
			respond.Error(w, http.StatusBadRequest, "bad-limit", "invalid_input", "limit must be 1..50")
			return
		}
		limit = parsed
	}

	var requestID, modelVersion string
	var items []map[string]any
	if h.Mock {
		name := "recommendations.full.json"
		if userID == "u2" {
			name = "recommendations.partial.json"
		}
		body, err := os.ReadFile(filepath.Join(h.ExamplesDir, name))
		if err != nil {
			respond.Error(w, http.StatusInternalServerError, "missing-example", "example_not_found", err.Error())
			return
		}
		var example struct {
			RequestID string `json:"request_id"`
			Items     []struct {
				ItemID      string `json:"item_id"`
				Title       string `json:"title"`
				Kind        string `json:"recommendation_kind"`
				Explanation string `json:"explanation"`
				URL         string `json:"url"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &example); err != nil {
			respond.Error(w, http.StatusInternalServerError, "bad-example", "example_invalid", err.Error())
			return
		}
		requestID = example.RequestID
		modelVersion = h.ModelVersion
		for i, item := range example.Items {
			if i >= limit {
				break
			}
			items = append(items, map[string]any{
				"item_id": item.ItemID, "title": item.Title,
				"subtitle": "синтетика", "category": item.Kind,
				"score": 0.9 - float64(i)*0.05, "reason": item.Explanation,
				"price": 0, "currency": "RUB", "tags": []string{}, "url": item.URL,
			})
		}
	} else {
		profile := h.profileFor(userID)
		result, err := h.Store.Recommend(r.Context(), profile.ClientID, limit)
		if err != nil {
			respond.Error(w, http.StatusInternalServerError, "store", "store_error", err.Error())
			return
		}
		requestID = result.RequestID
		modelVersion = result.ModelVersion
		for _, item := range result.Items {
			items = append(items, map[string]any{
				"item_id": item.ItemID, "title": item.Title,
				"subtitle": item.Subtitle, "category": item.Category,
				"score": item.Score, "reason": item.Reason,
				"price": item.Price, "currency": item.Currency,
				"tags": []string{item.Subtitle, item.Category}, "url": item.URL,
			})
		}
	}
	if items == nil {
		items = []map[string]any{}
	}

	respond.JSON(w, http.StatusOK, map[string]any{
		"user_id": userID, "request_id": requestID, "model_version": modelVersion,
		"generated_at": time.Now().UTC().Format(time.RFC3339Nano),
		"items":        items, "total": len(items), "next_cursor": nil,
	})
}

func (h *Handler) Feedback(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"user_id"`
		ItemID string `json:"item_id"`
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "bad-feedback", "invalid_input", err.Error())
		return
	}
	if body.ItemID == "" {
		respond.Error(w, http.StatusBadRequest, "bad-feedback", "invalid_input", "item_id is required")
		return
	}
	if h.Mock {
		respond.JSON(w, http.StatusOK, map[string]any{"accepted": true, "mode": "mock"})
		return
	}
	profile := h.profileFor(body.UserID)
	if err := h.Store.Feedback(r.Context(), profile.ClientID, body.ItemID, body.Action); err != nil {
		respond.Error(w, http.StatusInternalServerError, "feedback", "store_error", err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	if h.Mock {
		respond.JSON(w, http.StatusOK, map[string]any{"status": "mock", "tables": map[string]int64{}})
		return
	}
	counts, err := h.Store.Stats(r.Context())
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "stats", "store_error", err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"status": "ok", "model_version": h.ModelVersion, "tables": counts,
	})
}

func (h *Handler) Runs(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	profile := h.profileFor(userID)
	if h.Mock {
		respond.JSON(w, http.StatusOK, map[string]any{"runs": []any{}, "user_id": userID})
		return
	}
	runs, err := h.Store.Runs(r.Context(), profile.ClientID, 10)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "runs", "store_error", err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"runs": runs, "user_id": userID, "client_id": profile.ClientID,
	})
}

func (h *Handler) NewRun(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if h.Mock {
		respond.JSON(w, http.StatusOK, map[string]any{"run_id": 0, "user_id": userID, "mode": "mock"})
		return
	}
	profile := h.profileFor(userID)
	id, err := h.Store.ForceNewRun(r.Context(), profile.ClientID)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "new-run", "store_error", err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{"run_id": id, "user_id": userID})
}
