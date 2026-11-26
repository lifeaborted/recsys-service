// Package auth — HTTP-обработчики демо-сессий.
package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"recsys/internal/auth"
	"recsys/internal/transport/http/respond"
)

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type registerRequest struct {
	Login       string `json:"login"`
	Password    string `json:"password"`
	ClientGroup string `json:"client_group"`
}

func Login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "bad-login", "invalid_input", err.Error())
		return
	}
	profile, token, ok := auth.Login(body.Login, body.Password)
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized", "invalid_credentials", "Invalid credentials")
		return
	}
	respond.JSON(w, http.StatusOK, map[string]any{
		"access_token": token,
		"expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"profile_id":   profile.ID,
	})
}

func Register(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "bad-register", "invalid_input", err.Error())
		return
	}
	profile, token, ok := auth.Register(body.Login, body.Password, body.ClientGroup)
	if !ok {
		respond.Error(w, http.StatusConflict, "conflict", "login_exists", "Login already exists")
		return
	}
	respond.JSON(w, http.StatusCreated, map[string]any{
		"access_token": token,
		"expires_at":   time.Now().Add(24 * time.Hour).Format(time.RFC3339),
		"profile_id":   profile.ID,
	})
}

func Me(w http.ResponseWriter, r *http.Request) {
	profile, ok := auth.Current(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized", "unauthorized", "Missing or invalid session")
		return
	}
	respond.JSON(w, http.StatusOK, profile)
}
