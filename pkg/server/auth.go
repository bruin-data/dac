package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

func passwordEqual(got, want string) bool {
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func (s *Server) authenticated(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if scheme, token, ok := strings.Cut(header, " "); ok && strings.EqualFold(scheme, "Bearer") {
		return passwordEqual(token, s.config.AdminPassword)
	}
	// HTTP Basic gives browsers a native login prompt and also authenticates
	// subsequent fetch and EventSource requests. The username is ignored.
	_, password, ok := r.BasicAuth()
	return ok && passwordEqual(password, s.config.AdminPassword)
}

func (s *Server) requirePassword(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.config.AdminPassword != "" {
			w.Header().Set("Cache-Control", "no-store")
			if !s.authenticated(r) {
				w.Header().Set("WWW-Authenticate", `Basic realm="dac", charset="UTF-8"`)
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
