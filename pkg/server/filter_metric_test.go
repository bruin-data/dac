package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
)

func TestFilterOptionMetricEndpoint(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "metrics.yml"), []byte(`name: Metrics
connection: warehouse
filters:
  - name: region
    type: select
    multiple: true
    options:
      query: SELECT region, ignored, total_sales FROM sales
      metric: {field: total_sales, format: "$,.0f"}
rows:
  - widgets:
      - {name: Help, type: text, content: demo}
`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		columns []query.ColumnInfo
		rows    [][]any
		status  int
	}{
		{"display column is named, not positional", []query.ColumnInfo{{Name: "region"}, {Name: "ignored"}, {Name: "total_sales"}}, [][]any{{"Europe", "not a label", 530.0}, {"APAC", "ignored", 0.0}, {"Other", "ignored", nil}, {"Europe", "ignored", 999.0}, {"Loss", "ignored", -5.0}}, 200},
		{"metric column matches case-insensitively", []query.ColumnInfo{{Name: "REGION"}, {Name: "IGNORED"}, {Name: "TOTAL_SALES"}}, [][]any{{"Europe", "not a label", 530.0}, {"APAC", "ignored", 0.0}, {"Other", "ignored", nil}, {"Europe", "ignored", 999.0}, {"Loss", "ignored", -5.0}}, 200},
		{"missing metric column fails", []query.ColumnInfo{{Name: "region"}}, [][]any{{"Europe"}}, 500},
		{"nested metric fails", []query.ColumnInfo{{Name: "region"}, {Name: "total_sales"}}, [][]any{{"Europe", []any{1}}}, 500},
		{"empty results preserve an empty options array", []query.ColumnInfo{{Name: "region"}, {Name: "total_sales"}}, nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(Config{DashboardDir: dir, TemplateName: "bruin"})
			if err != nil {
				t.Fatal(err)
			}
			s.backend = &mockBackend{result: &query.QueryResult{Columns: tc.columns, Rows: tc.rows}}
			w := httptest.NewRecorder()
			s.mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/dashboards/Metrics/filters/region/options", strings.NewReader(`{"filters":{"region":["Europe"]}}`)))
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status != 200 {
				return
			}
			var got struct {
				Values  []string       `json:"values"`
				Metrics map[string]any `json:"metric_values"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if tc.rows == nil {
				if got.Values == nil || len(got.Values) != 0 {
					t.Fatalf("expected empty array: %s", w.Body.String())
				}
				return
			}
			if !reflect.DeepEqual(got.Values, []string{"Europe", "APAC", "Other", "Loss"}) {
				t.Fatalf("values changed by metrics: %v", got.Values)
			}
			if !reflect.DeepEqual(got.Metrics, map[string]any{"Europe": 530.0, "APAC": 0.0, "Other": nil, "Loss": -5.0}) {
				t.Fatalf("metrics: %v", got.Metrics)
			}
		})
	}
}

func TestModelFilterOptions(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "sales"}, Dimensions: []sem.Dimension{{Name: "region", Type: "string"}}, Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)", Format: &sem.Format{Type: "currency", Currency: "EUR", Decimals: 2}}}}
	for _, tc := range []struct {
		name     string
		metric   dashboard.FilterOptionMetric
		decimals int
	}{
		{"inherited format", dashboard.FilterOptionMetric{Name: "revenue"}, 2},
		{"object inherits format", dashboard.FilterOptionMetric{Field: "revenue"}, 2},
		{"filter format override", dashboard.FilterOptionMetric{Field: "revenue", Format: "$,.0f"}, 2},
		{"structured format override", dashboard.FilterOptionMetric{Field: "revenue", Format: map[string]any{"type": "currency", "currency": "TRY", "decimals": 0}}, 2},
		{"zero decimals survive JSON", dashboard.FilterOptionMetric{Name: "revenue"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model.Metrics[0].Format.Decimals = tc.decimals
			d := &dashboard.Dashboard{Connection: "warehouse", Model: "sales_alias", Models: map[string]string{"sales_alias": "sales"}, Filters: []dashboard.Filter{{Name: "region", Type: "select", Options: &dashboard.FilterOptions{Dimension: "region", Metric: &tc.metric}}}}
			d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
			b := &mockBackend{result: &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "revenue"}}, Rows: [][]any{{"Europe", 530.0}, {"APAC", 850.0}}}}
			if err := ResolveFilterOptions(context.Background(), b, d, map[string]any{"region": "Europe"}); err != nil {
				t.Fatal(err)
			}
			o := d.Filters[0].Options
			if !reflect.DeepEqual(o.Values, []string{"Europe", "APAC"}) || !reflect.DeepEqual(o.MetricValues, map[string]any{"Europe": 530.0, "APAC": 850.0}) {
				t.Fatalf("options: %+v", o)
			}
			if o.MetricFormat == nil || o.MetricFormat.Currency != "EUR" || o.MetricFormat.Decimals == nil || *o.MetricFormat.Decimals != tc.decimals || !reflect.DeepEqual(*o.Metric, tc.metric) {
				t.Fatalf("model format or filter override changed: %+v", o)
			}
			data, err := json.Marshal(o.MetricFormat)
			if err != nil {
				t.Fatal(err)
			}
			var format map[string]any
			if err := json.Unmarshal(data, &format); err != nil || format["decimals"] != float64(tc.decimals) {
				t.Fatalf("precision lost in JSON: %s (%v)", data, err)
			}
			if len(b.calls) != 1 || b.calls[0].Connection != "warehouse" {
				t.Fatalf("calls: %+v", b.calls)
			}
			sql := b.calls[0].SQL
			if !strings.Contains(sql, "sum(amount)") || !strings.Contains(sql, "GROUP BY") || strings.Contains(sql, "Europe") {
				t.Fatalf("unexpected options SQL: %s", sql)
			}
		})
	}
}

func TestModelFilterOptionsWithoutMetric(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "sales"}, Dimensions: []sem.Dimension{{Name: "region", Type: "string"}}}
	d := &dashboard.Dashboard{Model: "sales", Filters: []dashboard.Filter{{Name: "region", Type: "select", Options: &dashboard.FilterOptions{Dimension: "region"}}}}
	d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
	b := &mockBackend{result: &query.QueryResult{Rows: [][]any{{"Europe"}, {"Europe"}, {"APAC"}}}}
	if err := ResolveFilterOptions(context.Background(), b, d, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Filters[0].Options.Values, []string{"Europe", "APAC"}) {
		t.Fatalf("options: %+v", d.Filters[0].Options)
	}
}

func TestQueryFilterOptionsDefaultMetric(t *testing.T) {
	for _, tc := range []struct {
		name    string
		columns []query.ColumnInfo
		rows    [][]any
		metrics map[string]any
		wantErr bool
	}{
		{"one column stays plain", []query.ColumnInfo{{Name: "region"}}, [][]any{{"Europe"}}, nil, false},
		{"two columns infer metric", []query.ColumnInfo{{Name: "region"}, {Name: "total_sales"}}, [][]any{{"Europe", 530.0}, {"Zero", 0.0}, {"Missing", nil}, {"Europe", 999.0}}, map[string]any{"Europe": 530.0, "Zero": 0.0, "Missing": nil}, false},
		{"three columns stay plain", []query.ColumnInfo{{Name: "region"}, {Name: "label"}, {Name: "total_sales"}}, [][]any{{"Europe", "Europe sales", 530.0}}, nil, false},
		{"empty results", []query.ColumnInfo{{Name: "region"}, {Name: "total_sales"}}, nil, map[string]any{}, false},
		{"non-scalar metric fails", []query.ColumnInfo{{Name: "region"}, {Name: "total_sales"}}, [][]any{{"Europe", []any{1}}}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &dashboard.Dashboard{Connection: "warehouse", Filters: []dashboard.Filter{{Name: "region", Type: "select", Options: &dashboard.FilterOptions{Query: "SELECT region, SUM(amount) AS total_sales FROM sales GROUP BY region"}}}}
			b := &mockBackend{result: &query.QueryResult{Columns: tc.columns, Rows: tc.rows}}
			err := ResolveFilterOptions(context.Background(), b, d, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error: %v", err)
			}
			if tc.wantErr {
				return
			}
			o := d.Filters[0].Options
			if !reflect.DeepEqual(o.MetricValues, tc.metrics) || o.Metric != nil {
				t.Fatalf("unexpected options: %+v", o)
			}
			if len(b.calls) != 1 || (len(o.Values) > 0 && o.Values[0] != "Europe") {
				t.Fatalf("unexpected calls or selection values: %+v, %v", b.calls, o.Values)
			}
		})
	}
}
