package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bruin-data/dac/pkg/config"
)

// requireAdmin is middleware that checks the Authorization header against the
// configured admin password. If no password is configured, admin endpoints
// are disabled entirely.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.AdminPassword == "" {
			writeError(w, http.StatusForbidden, "admin not enabled")
			return
		}

		// Require an explicit token for mutations and arbitrary SQL. Browser
		// Basic credentials are ambient and must not authorize cross-site writes.
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || !passwordEqual(token, s.config.AdminPassword) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		next(w, r)
	}
}

// handleAdminLogin validates the provided password.
func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if s.config.AdminPassword == "" {
		writeError(w, http.StatusForbidden, "admin not enabled")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !passwordEqual(req.Password, s.config.AdminPassword) {
		writeError(w, http.StatusUnauthorized, "invalid password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminListConnections returns all connections grouped by type for the
// default environment.
func (s *Server) handleAdminListConnections(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.config.ConfigFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	env, err := cfg.GetEnvironment(s.config.Environment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build a response that flattens Connection into a simple map per entry.
	result := make(map[string][]map[string]any)
	for connType, conns := range env.Connections {
		var entries []map[string]any
		for _, c := range conns {
			entry := map[string]any{"name": c.Name}
			for k, v := range c.Extra {
				entry[k] = v
			}
			entries = append(entries, entry)
		}
		result[connType] = entries
	}

	writeJSON(w, http.StatusOK, map[string]any{"connections": result})
}

type createConnectionRequest struct {
	Type   string         `json:"type"`
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields"`
}

// handleAdminCreateConnection adds a new connection to the config.
func (s *Server) handleAdminCreateConnection(w http.ResponseWriter, r *http.Request) {
	var req createConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "type and name are required")
		return
	}

	cfg, err := config.Load(s.config.ConfigFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	env, err := cfg.GetEnvironment(s.config.Environment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Check for duplicate name within the same type.
	for _, c := range env.Connections[req.Type] {
		if c.Name == req.Name {
			writeError(w, http.StatusConflict, "connection already exists: "+req.Name)
			return
		}
	}

	conn := config.Connection{
		Name:  req.Name,
		Extra: req.Fields,
	}

	if env.Connections == nil {
		env.Connections = make(map[string][]config.Connection)
	}
	env.Connections[req.Type] = append(env.Connections[req.Type], conn)

	// Write the modified environment back into the config.
	envName := s.config.Environment
	if envName == "" {
		envName = cfg.DefaultEnvironment
	}
	cfg.Environments[envName] = *env

	if err := cfg.Save(s.config.ConfigFile); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// handleAdminUpdateConnection updates an existing connection's fields.
func (s *Server) handleAdminUpdateConnection(w http.ResponseWriter, r *http.Request) {
	connType := r.PathValue("type")
	connName := r.PathValue("name")

	var req struct {
		Fields map[string]any `json:"fields"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cfg, err := config.Load(s.config.ConfigFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	env, err := cfg.GetEnvironment(s.config.Environment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	conns, ok := env.Connections[connType]
	if !ok {
		writeError(w, http.StatusNotFound, "connection type not found: "+connType)
		return
	}

	found := false
	for i, c := range conns {
		if c.Name == connName {
			if conns[i].Extra == nil {
				conns[i].Extra = make(map[string]any)
			}
			for k, v := range req.Fields {
				conns[i].Extra[k] = v
			}
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "connection not found: "+connName)
		return
	}

	env.Connections[connType] = conns

	envName := s.config.Environment
	if envName == "" {
		envName = cfg.DefaultEnvironment
	}
	cfg.Environments[envName] = *env

	if err := cfg.Save(s.config.ConfigFile); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminDeleteConnection removes a connection from the config.
func (s *Server) handleAdminDeleteConnection(w http.ResponseWriter, r *http.Request) {
	connType := r.PathValue("type")
	connName := r.PathValue("name")

	cfg, err := config.Load(s.config.ConfigFile)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load config: "+err.Error())
		return
	}

	env, err := cfg.GetEnvironment(s.config.Environment)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	conns, ok := env.Connections[connType]
	if !ok {
		writeError(w, http.StatusNotFound, "connection type not found: "+connType)
		return
	}

	found := false
	for i, c := range conns {
		if c.Name == connName {
			env.Connections[connType] = append(conns[:i], conns[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		writeError(w, http.StatusNotFound, "connection not found: "+connName)
		return
	}

	envName := s.config.Environment
	if envName == "" {
		envName = cfg.DefaultEnvironment
	}
	cfg.Environments[envName] = *env

	if err := cfg.Save(s.config.ConfigFile); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAdminTestConnection tests a connection by running SELECT 1.
func (s *Server) handleAdminTestConnection(w http.ResponseWriter, r *http.Request) {
	connName := r.PathValue("name")

	_, err := s.backend.Execute(r.Context(), connName, "SELECT 1")
	if err != nil {
		writeError(w, http.StatusBadGateway, "connection test failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
