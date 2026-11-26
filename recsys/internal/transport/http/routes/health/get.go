package health

import (
	"context"
	"net/http"

	"recsys/internal/transport/http/respond"
)

// Get проверяет готовность: в mock-режиме всегда готов, в live — проверяет схему БД.
func Get(checker interface {
	Health(context.Context) error
}, mock bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !mock && (checker == nil || checker.Health(r.Context()) != nil) {
			respond.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
