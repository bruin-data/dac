package server

import (
	"encoding/json"
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

func filterSecurityServer(t *testing.T) (*Server, *mockBackend) {
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
	s, err := New(Config{DashboardDir: dir, TemplateName: "bruin", Frontend: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("dashboard app")}}})
	if err != nil {
		t.Fatal(err)
	}
	backend := &mockBackend{result: &query.QueryResult{Rows: [][]any{{1}}}}
	s.backend = backend
	s.loader.backend = backend
	return s, backend
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
	s, backend := filterSecurityServer(t)
	for _, suffix := range []string{"data", "stream", "widgets/r0-w0/query"} {
		for _, filters := range cases {
			body, _ := json.Marshal(map[string]any{"filters": filters})
			req := httptest.NewRequest("POST", "/api/v1/dashboards/Private/"+suffix, strings.NewReader(string(body)))
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

func TestQueryEndpointsAcceptClientDefaultsAndClearedInputs(t *testing.T) {
	s, backend := filterSecurityServer(t)
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
	// The engine formats lists and maps under scalar operators with %v, unquoted.
	job, _, _ = d.ResolveWidgetSemanticJob(&d.Rows[0].Widgets[1])
	if sql, _, _, err := compileSemanticJob(job, map[string]any{"country": []any{"x] OR 1=1 --"}}); err == nil {
		t.Fatalf("list under equals accepted: %s", sql)
	}
	job.Query.Filters[0].Value = map[string]any{"field": "{{ filters.country }}"}
	if sql, _, _, err := compileSemanticJob(job, map[string]any{"country": "x OR 1=1 --"}); err == nil {
		t.Fatalf("map under equals accepted: %s", sql)
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
