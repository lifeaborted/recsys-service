package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"recsys-migration/internal/config"
	"recsys-migration/internal/store"
)

// Handler реализует контракт, который ждёт фронтенд (src/api/client.ts).
type Handler struct {
	store *store.Store
	cfg   *config.Config
}

func New(s *store.Store, cfg *config.Config) *Handler {
	return &Handler{store: s, cfg: cfg}
}

type recommendationResponse struct {
	UserID       string       `json:"user_id"`
	GeneratedAt  string       `json:"generated_at"`
	ModelVersion string       `json:"model_version"`
	RequestID    string       `json:"request_id"`
	Items        []store.Item `json:"items"`
	Total        int          `json:"total"`
	NextCursor   *string      `json:"next_cursor"`
}

type feedbackPayload struct {
	UserID    string         `json:"user_id"`
	ItemID    string         `json:"item_id"`
	Action    string         `json:"action"`
	Scenario  string         `json:"scenario"`
	RequestID *string        `json:"request_id"`
	TS        string         `json:"ts"`
	Context   map[string]any `json:"context"`
}

// Routes возвращает mux с CORS-обёрткой.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /recommendations", h.recommendations)
	mux.HandleFunc("POST /recommendations/new", h.newRun)
	mux.HandleFunc("POST /feedback", h.feedback)
	mux.HandleFunc("GET /debug/stats", h.stats)
	mux.HandleFunc("GET /debug/runs", h.runs)
	return withCORS(mux)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	version, err := h.store.Health(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"model_version": version,
		"mode":          "live",
	})
}

func (h *Handler) recommendations(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "обязательный параметр user_id не задан")
		return
	}

	limit := 12
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "limit должен быть положительным числом")
			return
		}
		limit = parsed
	}
	if limit > 50 {
		limit = 50
	}
	// Сценарий сейчас не влияет на baseline, но принимается для совместимости.
	_ = r.URL.Query().Get("scenario")

	ref, err := h.store.ResolveUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result, err := h.store.Recommend(r.Context(), ref.ClientID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, recommendationResponse{
		UserID:       userID,
		GeneratedAt:  result.GeneratedAt.Format(time.RFC3339Nano),
		ModelVersion: result.ModelVersion,
		RequestID:    result.RequestID,
		Items:        result.Items,
		Total:        len(result.Items),
		NextCursor:   nil,
	})
}

func (h *Handler) newRun(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "обязательный параметр user_id не задан")
		return
	}
	ref, err := h.store.ResolveUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	run, err := h.store.ForceNewRun(r.Context(), ref.ClientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": run.RunID, "client_id": ref.ClientID,
		"items": run.Items, "count": len(run.Items),
	})
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	counts, err := h.store.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	version, err := h.store.Health(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "model_version": version, "tables": counts,
	})
}

func (h *Handler) runs(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "обязательный параметр user_id не задан")
		return
	}
	ref, err := h.store.ResolveUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}
	items, err := h.store.Runs(r.Context(), ref.ClientID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": userID, "client_id": ref.ClientID, "runs": items,
	})
}

func (h *Handler) feedback(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var payload feedbackPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if payload.UserID == "" || payload.ItemID == "" {
		writeError(w, http.StatusBadRequest, "обязательные поля user_id и item_id")
		return
	}

	ref, err := h.store.ResolveUser(r.Context(), payload.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.RecordFeedback(r.Context(), ref.ClientID, payload.ItemID, payload.Action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": true,
		"event_id": fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		"stored":   true,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"detail": message})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
