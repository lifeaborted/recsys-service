// Package respond writes JSON payloads and OpenAPI Error responses with X-Request-ID correlation.
package respond

import (
	"encoding/json"
	"net/http"

	"recsys/internal/logger"
)

const RequestIDHeader = "X-Request-ID"

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	RequestID string      `json:"request_id"`
	Error     errorDetail `json:"error"`
}

// RequestID extracts the active request ID from r's context or response header.
func RequestID(w http.ResponseWriter, r *http.Request) string {
	if r != nil {
		if id := logger.RequestIDFromContext(r.Context()); id != "" {
			return id
		}
	}
	if w != nil {
		if id := w.Header().Get(RequestIDHeader); id != "" {
			return id
		}
	}
	return "unknown"
}

// JSON serializes payload as application/json and ensures X-Request-ID header is present.
func JSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	reqID := RequestID(w, r)
	if w.Header().Get(RequestIDHeader) == "" && reqID != "" {
		w.Header().Set(RequestIDHeader, reqID)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// Error writes an OpenAPI Error JSON object and sets X-Request-ID on the response.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reqID := RequestID(w, r)
	if w.Header().Get(RequestIDHeader) == "" && reqID != "" {
		w.Header().Set(RequestIDHeader, reqID)
	}
	JSON(w, r, status, errorEnvelope{
		RequestID: reqID,
		Error: errorDetail{
			Code:    code,
			Message: message,
		},
	})
}
