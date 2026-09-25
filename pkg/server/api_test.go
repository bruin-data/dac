package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
)

// ---------------------------------------------------------------------------
// Mock backend
// ---------------------------------------------------------------------------

type mockBackend struct {
	result *query.QueryResult
	err    error
	mu     sync.Mutex
	calls  []mockCall
}

type mockCall struct {
	Connection string
	SQL        string
}

func (m *mockBackend) Execute(_ context.Context, conn string, sql string) (*query.QueryResult, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{Connection: conn, SQL: sql})
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// ---------------------------------------------------------------------------
// resolveWidgetJobs unit tests
// ---------------------------------------------------------------------------

func TestResolveWidgetJobs_RegularSQLWidgets(t *testing.T) {
	d := &dashboard.Dashboard{
		Name:       "test",
		Connection: "test-conn",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name: "Table",
				Type: "table",
				SQL:  "SELECT * FROM orders",
			}},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	assertEqual(t, jobs[0].SQL, "SELECT * FROM orders")
}

func TestResolveWidgetJobs_SkipsTextDivider(t *testing.T) {
	d := &dashboard.Dashboard{
		Name: "test",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{
				{Name: "txt", Type: "text", Content: "hi"},
				{Name: "div", Type: "divider"},
			},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs for text/divider, got %d", len(jobs))
	}
}

// A data-driven image widget now yields a data job like a table.
func TestResolveWidgetJobs_ImageGetsJob(t *testing.T) {
	d := &dashboard.Dashboard{
		Name: "test",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{
				{Name: "img", Type: "image", Src: "photo", SQL: "SELECT photo FROM listings"},
			},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job for a data-driven image widget, got %d", len(jobs))
	}
}

func TestResolveWidgetJobs_InlineDataNoBackend(t *testing.T) {
	d := &dashboard.Dashboard{
		Name: "test",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name: "Static",
				Type: "table",
				Data: &dashboard.WidgetData{
					Columns: []string{"region", "revenue"},
					Rows:    [][]any{{"EU", 100}, {"US", 200}},
				},
			}},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	assertEqual(t, jobs[0].SQL, "")
	if jobs[0].InlineData == nil {
		t.Fatal("expected job to carry inline data")
	}

	// A nil backend would panic if executed — inline data must short-circuit.
	wr := ExecuteWidgetQuery(context.Background(), nil, jobs[0])
	assertEqual(t, wr.Error, "")
	if len(wr.Columns) != 2 || wr.Columns[0].Name != "region" {
		t.Fatalf("unexpected columns: %+v", wr.Columns)
	}
	if len(wr.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(wr.Rows))
	}
}

func TestResolveWidgetJobs_JinjaFiltersRendered(t *testing.T) {
	d := &dashboard.Dashboard{
		Name:       "test",
		Connection: "conn",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name: "Filtered",
				Type: "table",
				SQL:  "SELECT * FROM orders WHERE region = '{{ filters.region }}'",
			}},
		}},
	}

	filters := map[string]any{"region": "US"}
	jobs, err := ResolveWidgetJobs(d, filters)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatal("expected 1 job")
	}
	assertContains(t, jobs[0].SQL, "region = 'US'")
	assertNotContains(t, jobs[0].SQL, "{{")
}

func TestResolveWidgetJobs_ExternalSemanticProjectDashboard(t *testing.T) {
	d, err := dashboard.LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	if err != nil {
		t.Fatal(err)
	}

	jobs, err := ResolveWidgetJobs(d, map[string]any{"country": "CA"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 6 {
		t.Fatalf("expected 6 jobs, got %d", len(jobs))
	}

	assertContains(t, jobs[0].SQL, "sum(amount) AS revenue")
	assertContains(t, jobs[1].SQL, "avg_order_value")
	assertContains(t, jobs[1].SQL, "country = 'CA'")
	assertContains(t, jobs[3].SQL, "date_trunc('month', order_date) AS order_date")
	assertContains(t, jobs[3].SQL, "ORDER BY order_date ASC")
	assertContains(t, jobs[4].SQL, "status = 'completed'")
	assertContains(t, jobs[4].SQL, "LIMIT 5")
	assertContains(t, jobs[5].SQL, "count(distinct order_id) AS order_count")
}

func TestBatchQuery_RegularDashboard(t *testing.T) {
	mock := &mockBackend{
		result: &query.QueryResult{
			Columns: []query.ColumnInfo{{Name: "total"}},
			Rows:    [][]any{{100}},
		},
	}
	s := &Server{
		backend: mock,
		loader:  &dashboardLoader{dir: "../../testdata/dashboards"},
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/data", s.handleBatchQuery)

	body := `{"filters":{}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/Sales%20Analytics/data", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	var resp struct {
		Widgets map[string]*WidgetQueryResult `json:"widgets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	// Sales dashboard has 11 widgets, all SQL-based.
	if len(resp.Widgets) == 0 {
		t.Fatal("expected widgets in response")
	}

	for id, wr := range resp.Widgets {
		if wr.Error != "" {
			t.Errorf("widget %q has error: %s", id, wr.Error)
		}
	}
}

func TestBatchQuery_ProjectSemanticDashboard(t *testing.T) {
	mock := &mockBackend{
		result: &query.QueryResult{
			Columns: []query.ColumnInfo{{Name: "value"}},
			Rows:    [][]any{{100}},
		},
	}
	s := &Server{
		backend: mock,
		paths:   dashboard.ResolveProjectPaths("../../testdata/project"),
		loader:  &dashboardLoader{dir: "../../testdata/project"},
	}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/data", s.handleBatchQuery)

	body := `{"filters":{"country":"CA"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/Semantic%20Sales/data", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)

	var resp struct {
		Widgets map[string]*WidgetQueryResult `json:"widgets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Widgets) != 6 {
		t.Fatalf("expected 6 widgets, got %d", len(resp.Widgets))
	}
	if len(mock.calls) != 6 {
		t.Fatalf("expected 6 backend calls, got %d", len(mock.calls))
	}
	for _, call := range mock.calls {
		assertNotContains(t, call.SQL, "{{")
		assertNotContains(t, call.SQL, "{%")
	}
}

func TestBatchQuery_NotFound(t *testing.T) {
	s := testServer(t)
	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/nonexistent/data", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusNotFound)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("missing %q in:\n  %s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("should not contain %q in:\n  %s", substr, s)
	}
}

func TestResolveWidgetJobs_TabsKeyedByName(t *testing.T) {
	d := &dashboard.Dashboard{
		Name:       "test",
		Connection: "test-conn",
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name: "Sales",
				Type: dashboard.WidgetTypeTabs,
				Tabs: []dashboard.Widget{
					{Name: "Revenue / Q1", Type: "table", SQL: "SELECT 1"},
					{Name: "Orders", Type: "table", SQL: "SELECT 2"},
				},
			}},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	assertEqual(t, jobs[0].ID, "r0-w0::Revenue / Q1")
	assertEqual(t, jobs[1].ID, "r0-w0::Orders")
}

func TestWidgetQuery_TabIDInPath(t *testing.T) {
	// A tab id carries the tab name, which may contain spaces or slashes; the
	// frontend path-escapes it and the route must still resolve the tab.
	dir := t.TempDir()
	yml := `name: T
rows:
  - widgets:
      - type: tabs
        tabs:
          - name: Revenue / Q1
            type: table
            data: { columns: [v], rows: [[7]] }
`
	if err := os.WriteFile(filepath.Join(dir, "t.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Server{backend: &mockBackend{}, loader: &dashboardLoader{dir: dir}}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("POST /api/v1/dashboards/{name}/widgets/{widgetId}/query", s.handleWidgetQuery)

	path := "/api/v1/dashboards/T/widgets/" + url.PathEscape("r0-w0::Revenue / Q1") + "/query"
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"filters":{}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	assertEqual(t, w.Code, http.StatusOK)
	var res WidgetQueryResult
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected the tab's inline row, got %+v", res)
	}
}

func TestResolveWidgetJobs_TabFiltersRendered(t *testing.T) {
	d := &dashboard.Dashboard{
		Name:       "test",
		Connection: "conn",
		Queries: map[string]dashboard.Query{
			"orders_q": {SQL: "SELECT * FROM orders WHERE region = '{{ filters.region }}'"},
		},
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Type: dashboard.WidgetTypeTabs,
				Tabs: []dashboard.Widget{
					{Name: "Inline", Type: "table", SQL: "SELECT * FROM sales WHERE region = '{{ filters.region }}'"},
					{Name: "Named", Type: "table", QueryRef: "orders_q"},
				},
			}},
		}},
	}

	jobs, err := ResolveWidgetJobs(d, map[string]any{"region": "US"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	for _, j := range jobs {
		assertContains(t, j.SQL, "region = 'US'")
		assertNotContains(t, j.SQL, "{{")
	}
}
