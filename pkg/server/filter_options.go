package server

import (
	"net/http"

	"github.com/bruin-data/dac/pkg/dashboard"
)

// handleFilterOptions resolves one filter independently of loading the dashboard
// and its widgets. Requests cannot override the configured SQL or connection.
func (s *Server) handleFilterOptions(w http.ResponseWriter, r *http.Request) {
	d, err := s.resolveDashboard(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return
	}
	for _, filter := range d.Filters {
		if filter.Name != r.PathValue("filter") {
			continue
		}
		if filter.Type != "select" {
			writeError(w, http.StatusBadRequest, "options are only available for select filters")
			return
		}
		optionsDashboard := &dashboard.Dashboard{Connection: d.Connection, Filters: []dashboard.Filter{filter}}
		if err := ResolveFilterOptions(r.Context(), s.backend, optionsDashboard); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		values := []string{}
		if options := optionsDashboard.Filters[0].Options; options != nil && options.Values != nil {
			values = options.Values
		}
		writeJSON(w, http.StatusOK, map[string]any{"values": values})
		return
	}
	writeError(w, http.StatusNotFound, "filter not found")
}
