package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/query"
)

func TestFilterOptions(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "options.yml"), []byte(`
name: Options Demo
connection: dashboard-connection
filters:
  - name: region
    type: select
    options:
      values: []
      query: SELECT region, ignored FROM regions ORDER BY region
  - name: mixed
    type: select
    options:
      values: [All, Europe]
      query: SELECT region FROM missing_table
  - name: status
    type: select
    multiple: true
    options:
      query: SELECT status FROM statuses
      connection: options-connection
  - name: static
    type: select
    options:
      values: [All, Europe]
  - name: empty
    type: select
  - name: search
    type: text
rows:
  - widgets:
      - name: Help
        type: text
        content: demo
`), 0600)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name       string
		path       string
		rows       [][]any
		err        error
		status     int
		values     []string
		connection string
		sql        string
	}{
		{name: "empty static list executes query; first column, deduplication and scalars", path: "Options%20Demo/filters/region/options",
			rows:   [][]any{{"Europe", "ignored"}, {"APAC", 99}, {nil}, {}, {float64(1000001)}, {true}, {"APAC"}},
			status: 200, values: []string{"Europe", "APAC", "1000001", "true"},
			connection: "dashboard-connection", sql: "SELECT region, ignored FROM regions ORDER BY region"},
		{name: "multi-select connection override", path: "Options%20Demo/filters/status/options",
			rows: [][]any{{"pending"}}, status: 200, values: []string{"pending"},
			connection: "options-connection", sql: "SELECT status FROM statuses"},
		{name: "empty query result", path: "Options%20Demo/filters/status/options",
			status: 200, values: []string{}, connection: "options-connection", sql: "SELECT status FROM statuses"},
		{name: "static values avoid backend", path: "Options%20Demo/filters/static/options", status: 200, values: []string{"All", "Europe"}},
		{name: "static values override failing query", path: "Options%20Demo/filters/mixed/options", err: errors.New("query must not execute"), status: 200, values: []string{"All", "Europe"}},
		{name: "no options returns array", path: "Options%20Demo/filters/empty/options", status: 200, values: []string{}},
		{name: "missing dashboard", path: "missing/filters/region/options", status: 404},
		{name: "missing filter", path: "Options%20Demo/filters/missing/options", status: 404},
		{name: "non-select", path: "Options%20Demo/filters/search/options", status: 400},
		{name: "query failure", path: "Options%20Demo/filters/status/options", err: errors.New("database unavailable"), status: 500,
			connection: "options-connection", sql: "SELECT status FROM statuses"},
		{name: "non-scalar result", path: "Options%20Demo/filters/status/options", rows: [][]any{{map[string]any{"nested": true}}}, status: 500,
			connection: "options-connection", sql: "SELECT status FROM statuses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(Config{DashboardDir: dir, TemplateName: "bruin"})
			if err != nil {
				t.Fatal(err)
			}
			backend := &mockBackend{result: &query.QueryResult{Rows: tc.rows}, err: tc.err}
			s.backend = backend
			s.loader.backend = backend
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/"+tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.status == http.StatusOK {
				var resp struct {
					Values []string `json:"values"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(resp.Values, tc.values) {
					t.Errorf("values %v, want %v", resp.Values, tc.values)
				}
			}
			if tc.sql == "" {
				if len(backend.calls) != 0 {
					t.Errorf("unexpected backend calls: %v", backend.calls)
				}
			} else if len(backend.calls) != 1 || backend.calls[0] != (mockCall{Connection: tc.connection, SQL: tc.sql}) {
				t.Errorf("unexpected backend calls: %v", backend.calls)
			}
		})
	}
}

func TestFilterOptionSelectionsAppliedToWidgets(t *testing.T) {
	dir := t.TempDir()
	definition := `name: Selection Demo
connection: warehouse
filters:
  - name: region
    type: select
    default: North
    options:
      values: []
      query: SELECT region FROM customers
  - name: status
    type: select
    multiple: true
    default: [active]
    options:
      values: [active, pending]
      query: SELECT status FROM missing_table
rows:
  - widgets:
      - name: Orders
        type: table
        sql: |
          SELECT * FROM orders
          WHERE region = '{{ filters.region }}'
            AND status IN ('{{ filters.status | join("','") }}')
`
	if err := os.WriteFile(filepath.Join(dir, "selection.yml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		body   string
		status int
	}{
		{"query-derived region and static multi-selection", `{"filters":{"region":"South","status":["active","pending"]}}`, 200},
		{"ignored query cannot expand static allowlist", `{"filters":{"region":"South","status":["archived"]}}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(Config{DashboardDir: dir, TemplateName: "bruin"})
			if err != nil {
				t.Fatal(err)
			}
			backend := &mockBackend{result: &query.QueryResult{Rows: [][]any{{"South"}}}}
			s.backend = backend
			s.loader.backend = backend
			// Resolving options must not mutate the definition used by widget requests.
			options := httptest.NewRecorder()
			s.mux.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/v1/dashboards/Selection%20Demo/filters/region/options", nil))
			if options.Code != 200 {
				t.Fatalf("options: %s", options.Body.String())
			}
			backend.calls = nil
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/Selection%20Demo/widgets/r0-w0/query", strings.NewReader(tc.body)))
			if w.Code != tc.status {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
			if tc.status == 200 {
				if len(backend.calls) != 1 {
					t.Fatalf("backend calls = %v", backend.calls)
				}
				assertContains(t, backend.calls[0].SQL, "region = 'South'")
				assertContains(t, backend.calls[0].SQL, "status IN ('active','pending')")
			} else if len(backend.calls) != 0 {
				t.Fatalf("invalid selection executed SQL: %v", backend.calls)
			}
		})
	}
}
