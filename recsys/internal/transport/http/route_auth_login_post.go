package transporthttp

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"recsys/internal/auth"
	"recsys/internal/transport/http/respond"
)

type loginRequestDTO struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type loginResponseDTO struct {
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
	ProfileID   string `json:"profile_id"`
}

func (s *Server) handlePostAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "auth_unavailable", "Auth service unavailable")
		return
	}
	var req loginRequestDTO
	if err := decodeStrictJSON(r, &req); err != nil {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Malformed login payload")
		return
	}
	if strings.TrimSpace(req.Login) == "" || req.Password == "" {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Login and password are required")
		return
	}

	profile, sess, err := s.auth.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			respond.Error(w, r, http.StatusUnauthorized, "invalid_credentials", "Invalid credentials")
			return
		}
		respond.Error(w, r, http.StatusInternalServerError, "internal_error", "Authentication failed")
		return
	}

	respond.JSON(w, r, http.StatusOK, loginResponseDTO{
		AccessToken: sess.AccessToken,
		ExpiresAt:   sess.ExpiresAt.UTC().Format(time.RFC3339),
		ProfileID:   profile.ID,
	})
}
