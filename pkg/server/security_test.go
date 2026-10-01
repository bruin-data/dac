package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
)

func securityServer(t *testing.T, password string) (*Server, *mockBackend) {
	t.Helper()
	dir := t.TempDir()
	definition := `name: Private
connection: warehouse
filters:
  - name: day
    type: date
    default: "2024-10-01"
  - name: dates
    type: date-range
    default: last_7_days
  - name: amount
    type: number
    default: 10
  - name: search
    type: text
    default: ""
  - name: region
    type: select
    default: US
    options:
      values: [US, EU]
  - name: platforms
    type: select
    multiple: true
    default: [ios]
    options:
      values: [ios, android]
rows:
  - widgets:
      - name: Results
        type: table
        sql: |
          SELECT DATE('{{ filters.day }}'), {{ filters.amount }}
          WHERE region = '{{ filters.region }}'
          AND name LIKE '%{{ filters.search }}%'
          AND date >= '{{ filters.dates.start }}'
          AND platform IN ('{{ filters.platforms | join("','") }}')
`
	if err := os.WriteFile(filepath.Join(dir, "private.yml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{DashboardDir: dir, AdminPassword: password, TemplateName: "bruin", Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("dashboard app")}}})
	if err != nil {
		t.Fatal(err)
	}
	backend := &mockBackend{result: &query.QueryResult{Rows: [][]any{{1}}}}
	s.backend = backend
	s.loader.backend = backend
	return s, backend
}

func TestPasswordProtectsEveryRoute(t *testing.T) {
	s, backend := securityServer(t, "secret")
	routes := []struct{ method, path string }{
		{"GET", "/"}, {"GET", "/dashboards/Private"},
		{"GET", "/api/v1/dashboards"}, {"GET", "/api/v1/dashboards/Private"},
		{"GET", "/api/v1/dashboards/Private/raw"}, {"POST", "/api/v1/dashboards/Private/data"},
		{"POST", "/api/v1/dashboards/Private/stream"}, {"POST", "/api/v1/dashboards/Private/widgets/r0-w0/query"},
		{"POST", "/api/v1/query"}, {"GET", "/api/v1/events"},
		{"GET", "/api/v1/config"}, {"GET", "/api/v1/themes"}, {"GET", "/api/v1/themes/bruin"},
		{"POST", "/api/v1/admin/login"}, {"GET", "/api/v1/admin/connections"},
	}
	for _, route := range routes {
		for _, authorization := range []string{"", "Bearer garbage", "secret", "Basic c2VjcmV0", "Bearer secret extra"} {
			t.Run(route.path+"/"+authorization, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"filters":{}}`))
				req.Header.Set("Authorization", authorization)
				w := httptest.NewRecorder()
				s.mux.ServeHTTP(w, req)
				if w.Code != http.StatusUnauthorized {
					t.Fatalf("got %d: %s", w.Code, w.Body.String())
				}
				if w.Header().Get("WWW-Authenticate") == "" {
					t.Fatal("missing browser challenge")
				}
			})
		}
	}
	if len(backend.calls) != 0 {
		t.Fatal("unauthenticated request executed SQL")
	}
}

func TestAuthenticatedDashboardRequests(t *testing.T) {
	s, _ := securityServer(t, "secret")
	for _, auth := range []string{"basic", "bearer"} {
		for _, path := range []string{"/", "/api/v1/dashboards", "/api/v1/dashboards/Private", "/api/v1/dashboards/Private/raw", "/api/v1/dashboards/Private/data", "/api/v1/dashboards/Private/stream", "/api/v1/dashboards/Private/widgets/r0-w0/query"} {
			t.Run(auth+path, func(t *testing.T) {
				method := "GET"
				if strings.HasSuffix(path, "/data") || strings.HasSuffix(path, "/stream") || strings.HasSuffix(path, "/query") {
					method = "POST"
				}
				req := httptest.NewRequest(method, path, strings.NewReader(`{"filters":{}}`))
				if auth == "basic" {
					req.SetBasicAuth("viewer", "secret")
				} else {
					req.Header.Set("Authorization", "Bearer secret")
				}
				w := httptest.NewRecorder()
				s.mux.ServeHTTP(w, req)
				if w.Code != 200 {
					t.Fatalf("got %d: %s", w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestArbitrarySQLRequiresExplicitAdminToken(t *testing.T) {
	for _, password := range []string{"", "secret"} {
		s, backend := securityServer(t, password)
		for _, auth := range []string{"none", "basic", "bearer"} {
			req := httptest.NewRequest("POST", "/api/v1/query", strings.NewReader(`{"connection":"warehouse","sql":"SELECT 1"}`))
			if auth == "basic" {
				req.SetBasicAuth("viewer", password)
			}
			if auth == "bearer" {
				req.Header.Set("Authorization", "Bearer "+password)
			}
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, req)
			want := 401
			if password == "" {
				want = 403
			} else if auth == "bearer" {
				want = 200
			}
			if w.Code != want {
				t.Errorf("password configured=%v auth=%s: got %d, want %d", password != "", auth, w.Code, want)
			}
		}
		wantCalls := 0
		if password != "" {
			wantCalls = 1
		}
		if len(backend.calls) != wantCalls {
			t.Errorf("got %d backend calls, want %d", len(backend.calls), wantCalls)
		}
	}
}

func TestQueryEndpointsRejectInvalidFiltersBeforeExecution(t *testing.T) {
	cases := []map[string]any{
		{"day": "2024-10-01'--"}, {"day": "2024-02-30"}, {"day": map[string]any{"start": "2024-10-01"}},
		{"amount": "0 OR 1=1"}, {"amount": true}, {"amount": []any{}},
		{"search": "' UNION SELECT secret FROM private --"}, {"search": `a\`}, {"search": []any{"a"}},
		{"region": "unknown"}, {"region": []any{"US"}},
		{"platforms": []any{"ios", "x') OR 1=1 --"}}, {"platforms": "ios"},
		{"dates": map[string]any{"start": "2024-10-01'--", "end": "2024-10-02"}},
		{"dates": map[string]any{"start": "2024-10-03", "end": "2024-10-02"}},
		{"dates": map[string]any{"start": "2024-10-01", "end": "2024-10-02", "extra": "x"}},
		{"undeclared": "x"},
	}
	for _, password := range []string{"", "secret"} {
		s, backend := securityServer(t, password)
		for _, suffix := range []string{"data", "stream", "widgets/r0-w0/query"} {
			for _, filters := range cases {
				body, _ := json.Marshal(map[string]any{"filters": filters})
				req := httptest.NewRequest("POST", "/api/v1/dashboards/Private/"+suffix, strings.NewReader(string(body)))
				if password != "" {
					req.Header.Set("Authorization", "Bearer "+password)
				}
				w := httptest.NewRecorder()
				s.mux.ServeHTTP(w, req)
				if w.Code != 400 {
					t.Errorf("%s %s: got %d: %s", suffix, body, w.Code, w.Body.String())
				}
				if strings.Contains(w.Body.String(), `"query"`) {
					t.Error("invalid request leaked SQL")
				}
			}
		}
		if len(backend.calls) != 0 {
			t.Fatalf("invalid filters executed %d queries", len(backend.calls))
		}
	}
}

func TestQueryEndpointsAcceptClientDefaultsAndClearedInputs(t *testing.T) {
	s, backend := securityServer(t, "")
	// The frontend echoes raw YAML defaults and sends "" for cleared inputs.
	for _, filters := range []map[string]any{
		{"day": "TODAY-1"}, {"day": ""}, {"region": ""},
	} {
		body, _ := json.Marshal(map[string]any{"filters": filters})
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/dashboards/Private/data", strings.NewReader(string(body))))
		if w.Code != 200 {
			t.Errorf("%s: got %d: %s", body, w.Code, w.Body.String())
		}
	}
	if len(backend.calls) == 0 {
		t.Fatal("no queries executed")
	}
}

func TestSemanticAuthorLiteralsMayContainQuotes(t *testing.T) {
	d, err := dashboard.LoadOneByName("../../testdata/project", "Semantic Sales")
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := d.ResolveWidgetSemanticJob(&d.Rows[0].Widgets[1])
	if err != nil {
		t.Fatal(err)
	}
	job.Query.Filters[0].Value = "Kids' Toys"
	sql, _, _, err := compileSemanticJob(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "'Kids'' Toys'") {
		t.Fatalf("author literal not escaped: %s", sql)
	}
}

func TestSemanticQueriesRejectUnsafeInterpolation(t *testing.T) {
	for _, expression := range []bool{false, true} {
		for _, value := range []any{"US' OR 1=1 --", `US\`, []any{"US", "x') OR 1=1 --"}} {
			t.Run("semantic", func(t *testing.T) {
				d, err := dashboard.LoadOneByName("../../testdata/project", "Semantic Sales")
				if err != nil {
					t.Fatal(err)
				}
				job, _, err := d.ResolveWidgetSemanticJob(&d.Rows[0].Widgets[1])
				if err != nil {
					t.Fatal(err)
				}
				if expression {
					job.Query.Filters = []sem.Filter{{Expression: "country = '{{ filters.country }}'"}}
				}
				if sql, _, _, err := compileSemanticJob(job, map[string]any{"country": value}); err == nil {
					t.Fatalf("unsafe semantic SQL accepted: %s", sql)
				}
			})
		}
	}
	d, err := dashboard.LoadOneByName("../../testdata/project", "Semantic Sales")
	if err != nil {
		t.Fatal(err)
	}
	job, _, err := d.ResolveWidgetSemanticJob(&d.Rows[0].Widgets[1])
	if err != nil {
		t.Fatal(err)
	}
	job.Query.Filters[0].Value = `{{ filters.country | replace("X", "'") }}`
	if sql, _, _, err := compileSemanticJob(job, map[string]any{"country": "X OR 1=1 --"}); err == nil {
		t.Fatalf("unsafe transformed semantic value accepted: %s", sql)
	}
	job.Query.Filters = []sem.Filter{{Expression: "{{ filters.country }}"}}
	if sql, _, _, err := compileSemanticJob(job, map[string]any{"country": "true OR true"}); err == nil {
		t.Fatalf("raw SQL semantic expression accepted: %s", sql)
	}
}

func TestNamedQueriesAndTabsRejectUnsafeInterpolation(t *testing.T) {
	for _, sql := range []string{"SELECT '{{ filters.value }}'", "SELECT * FROM {{ filters.value }}"} {
		d := &dashboard.Dashboard{
			Filters: []dashboard.Filter{{Name: "value", Type: "text"}},
			Queries: map[string]dashboard.Query{"named": {SQL: sql}},
			Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
				Type: dashboard.WidgetTypeTabs,
				Tabs: []dashboard.Widget{{Name: "Named", Type: "table", QueryRef: "named"}},
			}}}},
		}
		for _, value := range []string{"x' OR 1=1 --", "private UNION SELECT * FROM secrets"} {
			_, err := ResolveWidgetJobs(d, map[string]any{"value": value})
			if (strings.Contains(value, "'") || strings.Contains(sql, "FROM")) && err == nil {
				t.Fatalf("unsafe tab accepted: %s, %s", sql, value)
			}
		}
	}
}
