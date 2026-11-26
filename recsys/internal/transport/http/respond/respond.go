// Package respond — единый формат JSON-ответов и ошибок OpenAPI.
package respond

import (
	"encoding/json"
	"net/http"
)

func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func Error(w http.ResponseWriter, status int, requestID, code, message string) {
	JSON(w, status, map[string]any{
		"request_id": requestID,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func Raw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
