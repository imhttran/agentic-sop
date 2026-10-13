package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

type ctxKey int

const csrfKey ctxKey = iota

const (
	csrfCookie = "sop_ctrl_csrf"
)

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// csrfToken returns the per-session CSRF token attached by csrfProtect.
func csrfToken(r *http.Request) string {
	if v, ok := r.Context().Value(csrfKey).(string); ok {
		return v
	}
	return ""
}

// csrfProtect issues a CSRF cookie and rejects state-changing requests whose
// token doesn't match (FR: state-changing requests use POST + CSRF).
func csrfProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(csrfCookie); err == nil {
			token = c.Value
		}
		if token == "" {
			token = newToken()
			http.SetCookie(w, &http.Cookie{
				Name: csrfCookie, Value: token, Path: "/",
				HttpOnly: true, Secure: isHTTPS(r), SameSite: http.SameSiteStrictMode,
			})
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			got := r.FormValue("csrf")
			if got == "" {
				got = r.Header.Get("X-CSRF-Token")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey, token)))
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}
