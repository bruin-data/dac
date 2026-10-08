package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{
		DashboardDir: "../../testdata/dashboards",
		TemplateName: "bruin",
		Port:         0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func assertEqual(t *testing.T, got, want any) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestListDashboards(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	var resp struct {
		Dashboards []json.RawMessage `json:"dashboards"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	// 6 YAML + 2 TSX = 8
	assertEqual(t, len(resp.Dashboards), 8)
}

func TestGetDashboard(t *testing.T) {
	s := testServer(t)

	t.Run("existing dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/Sales%20Analytics", nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, req)

		assertEqual(t, w.Code, http.StatusOK)

		var resp map[string]any
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		assertEqual(t, resp["name"], "Sales Analytics")
	})

	t.Run("nonexistent dashboard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/nonexistent", nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, req)

		assertEqual(t, w.Code, http.StatusNotFound)
	})
}

func TestGetDashboardRaw(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/Sales%20Analytics/raw", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/yaml") {
		t.Errorf("content-type = %q, want text/yaml", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "name: Sales Analytics") {
		t.Errorf("body does not contain expected YAML name field")
	}
}

func TestConfig(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["template"]; !ok {
		t.Error("response missing 'template' field")
	}
	assertEqual(t, resp["template"], "bruin")
}

func TestListThemes(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/themes", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	var resp struct {
		Themes []any `json:"themes"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Themes) == 0 {
		t.Error("expected at least one theme")
	}
}

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(next)

	tests := []struct {
		name       string
		method     string
		origin     string
		wantStatus int
		wantOrigin string
	}{
		{name: "no origin", method: http.MethodGet, wantStatus: http.StatusOK},
		{name: "same origin", method: http.MethodPost, origin: "http://localhost:8321", wantStatus: http.StatusOK},
		{name: "VS Code webview", method: http.MethodPost, origin: "vscode-webview://dashboard-id", wantStatus: http.StatusOK, wantOrigin: "vscode-webview://dashboard-id"},
		{name: "VS Code preflight", method: http.MethodOptions, origin: "vscode-webview://dashboard-id", wantStatus: http.StatusNoContent, wantOrigin: "vscode-webview://dashboard-id"},
		{name: "arbitrary website", method: http.MethodPost, origin: "https://evil.example", wantStatus: http.StatusForbidden},
		{name: "spoofed scheme", method: http.MethodPost, origin: "vscode-webview.example://dashboard-id", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "http://localhost:8321/api/v1/query", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			assertEqual(t, w.Code, tt.wantStatus)
			assertEqual(t, w.Header().Get("Access-Control-Allow-Origin"), tt.wantOrigin)
		})
	}
}

func TestNew_AllowsRegularDashboardsWithInvalidSemanticModels(t *testing.T) {
	projectDir := t.TempDir()

	dashboardsDir := filepath.Join(projectDir, "dashboards")
	semanticDir := filepath.Join(projectDir, "semantic")
	if err := os.MkdirAll(dashboardsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(semanticDir, 0o755); err != nil {
		t.Fatal(err)
	}

	regularDashboard := `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Regular Dashboard
rows:
  - widgets:
      - name: Notes
        type: text
        content: Hello
`
	invalidModel := `schema: v1
name: broken_sales
source:
  table: sales
dimensions:
  - name: revenue
    type: string
metrics:
  - name: revenue
    expression: sum(amount)
`

	if err := os.WriteFile(filepath.Join(dashboardsDir, "regular.yml"), []byte(regularDashboard), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(semanticDir, "broken-sales.yml"), []byte(invalidModel), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := New(Config{
		DashboardDir: projectDir,
		TemplateName: "bruin",
		Port:         0,
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboards", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)
}
