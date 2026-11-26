package auth

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Profile — демо-профиль, привязанный к клиенту БД.
type Profile struct {
	ID       string `json:"profile_id"`
	ClientID string `json:"client_id"`
	Group    string `json:"client_group"`
	Login    string `json:"login"`
}

type contextKey struct{}

var (
	mu               sync.Mutex
	fallbackClientID string
	profiles         = map[string]Profile{
		"demo-token-u1": {ID: "u1", ClientID: "DEMO-B2B", Group: "B2B", Login: "demo-b2b"},
		"demo-token-u2": {ID: "u2", ClientID: "DEMO-B2C", Group: "B2C", Login: "demo-b2c"},
	}
)

// Configure применяет соответствия профилей клиентам БД из конфига.
func Configure(clientIDs map[string]string, fallback string) {
	mu.Lock()
	defer mu.Unlock()
	fallbackClientID = fallback
	for token, profile := range profiles {
		if clientID := clientIDs[profile.ID]; clientID != "" {
			profile.ClientID = clientID
		} else if fallback != "" {
			profile.ClientID = fallback
		}
		profiles[token] = profile
	}
}

// Register создаёт демо-профиль. Возвращает false при занятом логине.
func Register(login, password, group string) (Profile, string, bool) {
	if login == "" || password == "" || (group != "B2B" && group != "B2C") {
		return Profile{}, "", false
	}
	mu.Lock()
	defer mu.Unlock()
	for _, profile := range profiles {
		if profile.Login == login {
			return Profile{}, "", false
		}
	}
	clientID := fallbackClientID
	if clientID == "" {
		clientID = "DEMO-" + group
	}
	profile := Profile{
		ID:       "reg-" + login,
		ClientID: clientID,
		Group:    group,
		Login:    login,
	}
	token := "demo-token-" + profile.ID
	profiles[token] = profile
	return profile, token, true
}

// Login возвращает токен демо-сессии для известной пары логин/пароль.
func Login(login, password string) (Profile, string, bool) {
	if password == "" {
		return Profile{}, "", false
	}
	mu.Lock()
	defer mu.Unlock()
	for _, profile := range profiles {
		if profile.Login == login {
			token := "demo-token-" + profile.ID
			return profile, token, true
		}
	}
	return Profile{}, "", false
}

// FromToken находит профиль по Bearer-токену.
func FromToken(token string) (Profile, bool) {
	mu.Lock()
	defer mu.Unlock()
	profile, ok := profiles[token]
	return profile, ok
}

// FromRequest достаёт токен из заголовка Authorization.
func FromRequest(r *http.Request) (Profile, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return Profile{}, false
	}
	return FromToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
}

// WithProfile кладёт профиль в контекст запроса.
func WithProfile(ctx context.Context, profile Profile) context.Context {
	return context.WithValue(ctx, contextKey{}, profile)
}

// Current возвращает профиль из контекста (используется обработчиками).
func Current(ctx context.Context) (Profile, bool) {
	profile, ok := ctx.Value(contextKey{}).(Profile)
	return profile, ok
}

// Require оборачивает обработчик проверкой Bearer-сессии.
func Require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, ok := FromRequest(r)
		if !ok {
			writeUnauthorized(w)
			return
		}
		next(w, r.WithContext(WithProfile(r.Context(), profile)))
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"request_id":"unauthorized","error":{"code":"unauthorized","message":"Missing or invalid session"}}`))
}
