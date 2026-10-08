package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

const (
	sessionCookieName = "capper_session"
	csrfCookieName    = "capper_csrf"
	csrfHeaderName    = "X-CSRF-Token"
)

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Token string `json:"token"`
			TTL   string `json:"ttl,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeBadRequest(w, err)
			return
		}
		if req.Token == "" {
			writeBadRequest(w, errMissingToken)
			return
		}
		pt, pid, err := s.ctrl.Store.IAM.Verify(req.Token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		csrf, _ := randomToken(16)
		ttl := 24 * time.Hour
		if req.TTL != "" {
			if d, err := time.ParseDuration(req.TTL); err == nil {
				ttl = d
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    req.Token,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(ttl.Seconds()),
		})
		http.SetCookie(w, &http.Cookie{
			Name:     csrfCookieName,
			Value:    csrf,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(ttl.Seconds()),
		})
		writeData(w, map[string]any{
			"principalType": pt,
			"principalId":   pid,
			"csrfToken":     csrf,
		}, nil)
	case http.MethodDelete:
		http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleAuthSessionInfo(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		pt, pid, err := s.ctrl.Store.IAM.Verify(c.Value)
		if err == nil {
			csrf := ""
			if cc, err := r.Cookie(csrfCookieName); err == nil {
				csrf = cc.Value
			}
			writeData(w, map[string]any{
				"authenticated": true,
				"principalType": pt,
				"principalId":   pid,
				"csrfToken":     csrf,
			}, nil)
			return
		}
	}
	writeData(w, map[string]any{"authenticated": false}, nil)
}

// issueSession mints a session for a principal: it issues a bearer token (the
// session cookie value, verified by the auth middleware) and sets the session +
// CSRF cookies. Returns the CSRF token for the client to echo on mutations.
func (s *Server) issueSession(w http.ResponseWriter, pt, pid string, ttl time.Duration) (string, error) {
	bearer, _, err := s.ctrl.Store.IAM.Issue("session", pt, pid, ttl)
	if err != nil {
		return "", err
	}
	csrf, _ := randomToken(16)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: bearer, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name: csrfCookieName, Value: csrf, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: int(ttl.Seconds()),
	})
	return csrf, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var errMissingToken = authError{"token is required"}

type authError struct{ msg string }

func (e authError) Error() string { return e.msg }
