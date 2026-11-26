package transporthttp

import (
	"errors"
	"net/http"
	"time"

	"recsys/internal/auth"
	"recsys/internal/domain"
	"recsys/internal/transport/http/respond"
)

type registerRequestDTO struct {
	Login       string  `json:"login"`
	Password    string  `json:"password"`
	ClientGroup string  `json:"client_group"`
	Industry    *string `json:"industry,omitempty"`
}

type registerResponseDTO struct {
	ProfileID   string             `json:"profile_id"`
	ClientGroup domain.ClientGroup `json:"client_group"`
	AccessToken string             `json:"access_token"`
	ExpiresAt   string             `json:"expires_at"`
}

func (s *Server) handlePostAuthRegister(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		respond.Error(w, r, http.StatusServiceUnavailable, "auth_unavailable", "Auth service unavailable")
		return
	}
	var req registerRequestDTO
	if err := decodeStrictJSON(r, &req); err != nil {
		respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Malformed registration payload")
		return
	}

	profile, sess, err := s.auth.Register(r.Context(), auth.RegisterInput{
		Login:       req.Login,
		Password:    req.Password,
		ClientGroup: domain.ClientGroup(req.ClientGroup),
		Industry:    req.Industry,
	})
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidInput):
			respond.Error(w, r, http.StatusBadRequest, "invalid_input", "Invalid registration data")
		case errors.Is(err, auth.ErrLoginExists):
			respond.Error(w, r, http.StatusConflict, "login_exists", "Login already exists")
		default:
			respond.Error(w, r, http.StatusInternalServerError, "internal_error", "Could not create account")
		}
		return
	}

	respond.JSON(w, r, http.StatusCreated, registerResponseDTO{
		ProfileID:   profile.ID,
		ClientGroup: profile.Group,
		AccessToken: sess.AccessToken,
		ExpiresAt:   sess.ExpiresAt.UTC().Format(time.RFC3339),
	})
}
