package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"gopkg.in/yaml.v3"
)

func semanticGrandTotalModel() *sem.Model {
	return &sem.Model{Name: "sales", Source: sem.Source{Table: "(SELECT * FROM orders WHERE year = '{{ filters.year }}') AS orders"}, Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "status", Type: "string"}}, Metrics: []sem.Metric{
		{Name: "revenue", Expression: "sum(revenue)"},
		{Name: "spend", Expression: "sum(ad_spend)"},
		{Name: "customers", Expression: "count(distinct customer_id)"},
		{Name: "average_order", Expression: "avg(revenue)"},
		{Name: "roas", Expression: "{revenue} / nullif({spend}, 0)"},
		{Name: "completed_revenue", Expression: "sum(revenue)", Filter: "status = 'completed'"},
	}, Segments: []sem.Segment{{Name: "completed", Filter: "status = 'completed'"}}}
}
func semanticGrandTotalDashboard() *dashboard.Dashboard {
	d := &dashboard.Dashboard{Filters: []dashboard.Filter{{Name: "year", Type: "text", Default: "2026"}, {Name: "regions", Type: "select", Multiple: true, Options: &dashboard.FilterOptions{Values: []string{"Europe", "Asia"}}}}, Name: "GrandTotal", Connection: "warehouse", Model: "sales", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
		Name: "Regions", Type: dashboard.WidgetTypeTable, GrandTotal: true,
		Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}}, MetricRefs: []string{"revenue", "spend", "customers", "average_order", "roas", "completed_revenue"},
		Filters: []dashboard.SemanticQueryFilter{{Dimension: "region", Operator: "in", Value: "{{ filters.regions }}"}}, Segments: []string{"completed"}, Sort: []dashboard.SemanticSort{{Name: "revenue", Direction: "desc"}}, Limit: 1,
	}}}}}
	model := semanticGrandTotalModel()
	d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
	return d
}
func TestSemanticGrandTotalReuseDefinitionsFiltersSegmentsAndConnection(t *testing.T) {
	d := semanticGrandTotalDashboard()
	w := &d.Rows[0].Widgets[0]
	job, _, err := d.ResolveWidgetSemanticJob(w)
	if err != nil {
		t.Fatal(err)
	}
	filters := map[string]any{"year": "2026", "regions": []any{"Europe", "Asia"}}
	before := job.Query
	sql, err := compileSemanticGrandTotalJob(job, filters)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"year = '2026'", "count(distinct customer_id)", "avg(revenue)", "nullif(", "AS roas", "status = 'completed'", "region IN ('Europe', 'Asia')", "completed_revenue"} {
		if !strings.Contains(sql, want) {
			t.Errorf("totals missing %q: %s", want, sql)
		}
	}
	upper := strings.ToUpper(sql)
	for _, forbidden := range []string{"GROUP BY", "ORDER BY", "LIMIT"} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("totals contains %s: %s", forbidden, sql)
		}
	}
	if len(job.Query.Dimensions) != len(before.Dimensions) || job.Query.Limit != 1 || len(job.Query.Sort) != 1 {
		t.Fatal("mutated detail query")
	}
	if job.Model.Source.Table != "(SELECT * FROM orders WHERE year = '{{ filters.year }}') AS orders" {
		t.Fatal("mutated source model")
	}
	jobs, err := ResolveWidgetJobs(d, filters)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].GrandTotalSQL == "" {
		t.Fatal("missing totals job")
	}
	if !strings.Contains(jobs[0].SQL, "GROUP BY") || !strings.Contains(jobs[0].SQL, "LIMIT 1") {
		t.Fatal(jobs[0].SQL)
	}
	if jobs[0].GrandTotalSQL != sql || jobs[0].Connection != "warehouse" {
		t.Fatal("totals job mismatch")
	}
}

func TestSemanticGrandTotalNamedQueryInTab(t *testing.T) {
	d := semanticGrandTotalDashboard()
	d.Queries = map[string]dashboard.Query{"by_region": {Model: "sales", Connection: "named-warehouse", Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}}, Metrics: []string{"revenue", "customers"}, Limit: 2}}
	d.Rows[0].Widgets = []dashboard.Widget{{Type: dashboard.WidgetTypeTabs, Name: "Results", Tabs: []dashboard.Widget{{Name: "Summary", Type: dashboard.WidgetTypeTable, QueryRef: "by_region", GrandTotal: true}}}}
	jobs, err := ResolveWidgetJobs(d, map[string]any{"year": "2026"})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].ID != "r0-w0::Summary" || jobs[0].GrandTotalSQL == "" {
		t.Fatalf("unexpected jobs %+v", jobs)
	}
	if jobs[0].Connection != "named-warehouse" {
		t.Fatal(jobs[0].Connection)
	}
	if strings.Contains(jobs[0].GrandTotalSQL, "LIMIT") {
		t.Fatal(jobs[0].GrandTotalSQL)
	}
}

func TestSemanticGrandTotalKeepJoinedDimensionFilters(t *testing.T) {
	customers := &sem.Model{Name: "customers", Source: sem.Source{Table: "customers"}, PrimaryKey: "customer_id", Dimensions: []sem.Dimension{{Name: "country", Type: "string"}}}
	orders := &sem.Model{Name: "orders", Source: sem.Source{Table: "orders"}, PrimaryKey: "order_id", Joins: []sem.Join{{Name: "customers", Relationship: "many_to_one", ForeignKey: "customer_id"}}, Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}}}
	job := &dashboard.SemanticJob{Model: orders, ModelName: "orders", Models: map[string]*sem.Model{"orders": orders, "customers": customers}, Query: sem.Query{Dimensions: []sem.DimensionRef{{Name: "customers.country"}}, Metrics: []string{"revenue"}, Filters: []sem.Filter{{Dimension: "customers.country", Operator: "equals", Value: "{{ filters.country }}"}}}}
	sql, err := compileSemanticGrandTotalJob(job, map[string]any{"country": "CA"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "JOIN") || !strings.Contains(sql, "country = 'CA'") {
		t.Fatal(sql)
	}
	if strings.Contains(sql, "GROUP BY") {
		t.Fatal(sql)
	}
}

type grandTotalBackend struct {
	mu              sync.Mutex
	calls           []mockCall
	grandTotalError bool
	manyRows        bool
	detailError     bool
	detailRows      [][]any
}

func (b *grandTotalBackend) Execute(_ context.Context, conn, sql string) (*query.QueryResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, mockCall{Connection: conn, SQL: sql})
	if strings.Contains(sql, "GROUP BY") {
		if b.detailError {
			return nil, errors.New("detail failed")
		}
		rows := b.detailRows
		if rows == nil {
			rows = [][]any{{"Europe", 2, 200, 2.66667}, {"Asia", 2, 400, 3}}
		}
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "customers"}, {Name: "average_order"}, {Name: "roas"}}, Rows: rows}, nil
	}
	if b.grandTotalError {
		return nil, errors.New("total failed")
	}
	rows := [][]any{{3, 320, 1600.0 / 550}}
	if b.manyRows {
		rows = append(rows, []any{99, 99, 99})
	}
	return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "customers"}, {Name: "average_order"}, {Name: "roas"}}, Rows: rows}, nil
}
func TestExecuteSemanticGrandTotalAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		b         *grandTotalBackend
		wantCalls int
		wantError string
	}{
		{name: "success", wantCalls: 2},
		{name: "totals failure fails widget", b: &grandTotalBackend{grandTotalError: true}, wantCalls: 2, wantError: "total failed"},
		{name: "multiple totals rows rejected", b: &grandTotalBackend{manyRows: true}, wantCalls: 2, wantError: "expected at most one grand-total row"},
		{name: "detail failure skips totals", b: &grandTotalBackend{detailError: true}, wantCalls: 1, wantError: "detail failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.b
			if b == nil {
				b = &grandTotalBackend{}
			}
			j := WidgetJob{SQL: "SELECT region FROM orders GROUP BY region", Connection: "warehouse", GrandTotalSQL: "SELECT count(distinct customer_id) FROM orders"}
			result := ExecuteWidgetQuery(context.Background(), b, j)
			if len(b.calls) != tc.wantCalls {
				t.Fatal(b.calls)
			}
			for _, call := range b.calls {
				if call.Connection != j.Connection {
					t.Fatal("detail and totals must use the same connection")
				}
			}
			if result.Error != tc.wantError {
				t.Fatalf("want error %q, got %q", tc.wantError, result.Error)
			}
			if tc.wantError != "" {
				if result.GrandTotal != nil || len(result.Rows) != 0 {
					t.Fatal("failed widgets must not return partial results")
				}
				if result.Query != b.calls[len(b.calls)-1].SQL {
					t.Fatal("error must identify the failed query")
				}
				return
			}
			if len(result.Rows) != 2 || result.GrandTotal == nil || result.GrandTotal.Rows[0][0] != 3 {
				t.Fatalf("unexpected detail or totals: %+v", result)
			}
		})
	}
}

func TestSemanticGrandTotalAllDataEndpoints(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "semantic"), 0755); err != nil {
		t.Fatal(err)
	}
	model := semanticGrandTotalModel()
	model.Source.Table = "orders"
	raw, err := yaml.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "semantic", "sales.yml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	raw = []byte(`name: GrandTotal
connection: warehouse
model: sales
rows:
 - widgets:
    - name: Regions
      type: table
      dimensions: [{name: region}]
      metrics: [customers, average_order, roas]
      grandTotal: true
`)
	if err := os.WriteFile(filepath.Join(dir, "totals.yml"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"data", "stream", "widgets/r0-w0/query"} {
		t.Run(endpoint, func(t *testing.T) {
			b := &grandTotalBackend{}
			s := &Server{backend: b, loader: &dashboardLoader{dir: dir}, mux: http.NewServeMux()}
			s.mux.HandleFunc("POST /api/v1/dashboards/{name}/data", s.handleBatchQuery)
			s.mux.HandleFunc("POST /api/v1/dashboards/{name}/stream", s.handleStreamQuery)
			s.mux.HandleFunc("POST /api/v1/dashboards/{name}/widgets/{widgetId}/query", s.handleWidgetQuery)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/GrandTotal/"+endpoint, strings.NewReader(`{"filters":{}}`))
			rec := httptest.NewRecorder()
			s.mux.ServeHTTP(rec, req)
			if rec.Code != 200 {
				t.Fatalf("%d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), `"grandTotal":`) || strings.Contains(rec.Body.String(), `"totals":`) {
				t.Fatalf("expected grandTotal response key: %s", rec.Body.String())
			}
			var result *WidgetQueryResult
			switch endpoint {
			case "data":
				var body struct {
					Widgets map[string]*WidgetQueryResult `json:"widgets"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				result = body.Widgets["r0-w0"]
			case "stream":
				var body struct {
					Data *WidgetQueryResult `json:"data"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				result = body.Data
			default:
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
			}
			if result == nil || result.GrandTotal == nil || len(result.GrandTotal.Rows) != 1 || result.GrandTotal.Rows[0][0] != float64(3) {
				t.Fatalf("missing totals: %s", rec.Body.String())
			}
			if len(b.calls) != 2 {
				t.Fatal(b.calls)
			}
		})
	}
}

func TestSemanticGrandTotalUseQueryConnectionDialect(t *testing.T) {
	for _, tc := range []struct{ dialect, literal string }{
		{"postgres", "'Kids'' Toys'"},
		{"google_cloud_platform", `"Kids' Toys"`},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), ".bruin.yml")
			config := "environments:\n  test:\n    connections:\n      " + tc.dialect + ":\n        - name: warehouse\n"
			if err := os.WriteFile(cfg, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			backend := query.NewCachedBackend(&query.BruinCLIBackend{ConfigFile: cfg}, 0)
			d := semanticGrandTotalDashboard()
			d.Rows[0].Widgets[0].Filters = []dashboard.SemanticQueryFilter{{Dimension: "region", Operator: "equals", Value: "Kids' Toys"}}
			jobs, err := ResolveWidgetJobs(d, map[string]any{"year": "2026"}, backend)
			if err != nil {
				t.Fatal(err)
			}
			sql := jobs[0].GrandTotalSQL
			if !strings.Contains(sql, "region = "+tc.literal) {
				t.Fatalf("wrong totals escaping: %s", sql)
			}
		})
	}
}

func TestSemanticGrandTotalIgnoreDisplayedRowsAndLimit(t *testing.T) {
	jobs, err := ResolveWidgetJobs(semanticGrandTotalDashboard(), map[string]any{"year": "2026", "regions": []any{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range []string{"Europe", `Europe\West`} {
		t.Run(region, func(t *testing.T) {
			backend := &grandTotalBackend{detailRows: [][]any{{region, 2, 200, 2.66667}}}
			result := ExecuteWidgetQuery(context.Background(), backend, jobs[0])
			if result.Error != "" {
				t.Fatal(result.Error)
			}
			if len(result.Rows) != 1 || result.GrandTotal == nil {
				t.Fatal("expected one displayed row and a grand total")
			}
			values := result.GrandTotal.Rows[0]
			if values[0] != 3 || values[1] != 320 || values[2] != 1600.0/550 {
				t.Fatalf("total must use all filtered records: %v", values)
			}
			if len(backend.calls) != 2 || backend.calls[1].SQL != jobs[0].GrandTotalSQL {
				t.Fatal("totals query must not depend on returned group values")
			}
		})
	}
}

func TestSemanticGrandTotalKeepRowFiltersButOmitHaving(t *testing.T) {
	for _, expression := range []string{"{revenue} < 1000", "SUM(revenue) < 1000", "{% if filters.small %}{revenue} < 1000{% else %}status = 'completed'{% endif %}"} {
		t.Run(expression, func(t *testing.T) {
			model := semanticGrandTotalModel()
			model.Source.Table = "orders"
			model.Segments = append(model.Segments, sem.Segment{Name: "small", Filter: expression})
			job := &dashboard.SemanticJob{Model: model, Query: sem.Query{Dimensions: []sem.DimensionRef{{Name: "region"}}, Metrics: []string{"revenue", "completed_revenue"}, Filters: []sem.Filter{{Expression: expression}, {Dimension: "status", Operator: "equals", Value: "completed"}}, Segments: []string{"small", "completed"}, Limit: 1}}
			filters := map[string]any{"small": true}
			detail, _, _, err := compileSemanticJob(job, filters)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(detail, "HAVING") {
				t.Fatal(detail)
			}
			sql, err := compileSemanticGrandTotalJob(job, filters)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(sql, "HAVING") || strings.Contains(sql, "< 1000") {
				t.Fatal(sql)
			}
			if !strings.Contains(sql, "WHERE status = 'completed'") || !strings.Contains(sql, "CASE WHEN status = 'completed'") {
				t.Fatal(sql)
			}
			if len(job.Query.Filters) != 2 || len(job.Query.Segments) != 2 || job.Query.Limit != 1 {
				t.Fatal("mutated detail query")
			}
		})
	}
}
