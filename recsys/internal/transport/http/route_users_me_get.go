package transporthttp

import (
	"net/http"

	"recsys/internal/auth"
	"recsys/internal/transport/http/respond"
)

func (s *Server) handleGetUsersMe(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, r, http.StatusUnauthorized, "unauthorized", "Missing or invalid session")
		return
	}
	respond.JSON(w, r, http.StatusOK, profile)
}
