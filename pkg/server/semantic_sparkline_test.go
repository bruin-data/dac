package server

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
)

type seriesBackend struct{}

func (seriesBackend) Execute(_ context.Context, _ string, sql string) (*query.QueryResult, error) {
	switch {
	case strings.HasSuffix(sql, ") grand_total"):
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "avg_order"}}, Rows: [][]any{{1.2}}}, nil
	case strings.Contains(sql, "AS region") && strings.Contains(sql, "AS day"):
		// Ordered by day, as the series query asks.
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "day"}, {Name: "avg_order"}}, Rows: [][]any{
			{"EU", "2026-09-01", 1.21}, {"US", "2026-09-01", 1.1}, {"EU", "2026-09-02", 0.91}, {"EU", "2026-09-03", 1.54},
			{"EU", "2026-09-04", nil}, // a NULL y isn't a point and doesn't count toward the limit
		}}, nil
	case strings.Contains(sql, "AS day"):
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "day"}, {Name: "avg_order"}}, Rows: [][]any{
			{"2026-09-01", 1.15}, {"2026-09-02", 0.95}, {"2026-09-03", 1.5}, {"2026-09-04", nil},
		}}, nil
	}
	return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "avg_order"}}, Rows: [][]any{{"EU", 1.14}, {"US", 1.1}, {"APAC", 0.8}}}, nil
}

func TestSemanticSparklineSeriesFillsEachRowAndTotal(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "orders"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "day", Type: "time"}},
		Metrics: []sem.Metric{
			{Name: "revenue", Expression: "sum(revenue)"}, {Name: "orders", Expression: "count(*)"},
			{Name: "avg_order", Expression: "{revenue} / nullif({orders}, 0)"},
		}}
	d := &dashboard.Dashboard{Name: "s", Connection: "warehouse", Model: "sales", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
		Name: "Regions", Type: dashboard.WidgetTypeTable, GrandTotal: true,
		Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}}, MetricRefs: []string{"avg_order"},
		Sort: []dashboard.SemanticSort{{Name: "avg_order", Direction: "desc"}},
		Columns: []dashboard.TableColumn{{Name: "region"}, {Name: "avg_order"}, {Name: "trend", Type: "sparkline", Limit: 2,
			X: &dashboard.AxisEncoding{Field: "day"}, Y: &dashboard.AxisEncoding{Field: "avg_order"}}},
	}}}}}
	d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
	if err := d.Rows[0].Widgets[0].ValidateSparklineSeries(d); err != nil {
		t.Fatal(err)
	}
	// A misspelled metric would leave the cell blank, so validation reports it.
	typo := *d
	typo.Rows = []dashboard.Row{{Widgets: []dashboard.Widget{d.Rows[0].Widgets[0]}}}
	typo.Rows[0].Widgets[0].Columns = []dashboard.TableColumn{{Name: "trend", Type: "sparkline",
		X: &dashboard.AxisEncoding{Field: "day"}, Y: &dashboard.AxisEncoding{Field: "avg_ordr"}}}
	if err := typo.Rows[0].Widgets[0].ValidateSparklineSeries(&typo); err == nil {
		t.Fatal("validation must catch a y.field that is not a model metric")
	}

	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	series := jobs[0].Series
	if len(series) != 1 || series[0].Keys != 1 {
		t.Fatalf("series = %+v", series)
	}
	// Each point is that day's ratio; sort and limit don't cut points.
	if sql := series[0].SQL; !strings.Contains(sql, "nullif(") || !strings.Contains(sql, "GROUP BY 1, 2") || strings.Contains(sql, "avg_order DESC") {
		t.Fatalf("series SQL: %s", sql)
	}

	wr := ExecuteWidgetQuery(context.Background(), seriesBackend{}, jobs[0])
	if wr.Error != "" {
		t.Fatal(wr.Error)
	}
	if wr.Columns[2].Name != "trend" {
		t.Fatalf("columns = %+v", wr.Columns)
	}
	want := [][]map[string]any{
		{{"day": "2026-09-02", "avg_order": 0.91}, {"day": "2026-09-03", "avg_order": 1.54}}, // last 2 of EU
		{{"day": "2026-09-01", "avg_order": 1.1}},
		{}, // no points for APAC
	}
	for i, row := range wr.Rows {
		if !reflect.DeepEqual(row[2], want[i]) {
			t.Errorf("row %v trend = %v, want %v", row[0], row[2], want[i])
		}
	}
	total := wr.GrandTotal.Rows[0][len(wr.GrandTotal.Rows[0])-1]
	if !reflect.DeepEqual(total, []map[string]any{{"day": "2026-09-02", "avg_order": 0.95}, {"day": "2026-09-03", "avg_order": 1.5}}) {
		t.Errorf("total trend = %v", total)
	}
}

type windowBackend struct{}

func (windowBackend) Execute(_ context.Context, _ string, sql string) (*query.QueryResult, error) {
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	if strings.Contains(sql, "AS avg_order") {
		// The series query: same dimensions, the sparkline metric, ordered by day.
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "day"}, {Name: "avg_order"}}, Rows: [][]any{
			{"EU", day(1), 1.21}, {"US", day(1), 1.1}, {"EU", day(2), 0.91}, {"US", day(2), 1.3}, {"EU", day(3), 1.54},
		}}, nil
	}
	return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "day"}, {Name: "revenue"}}, Rows: [][]any{
		{"EU", day(3), 300}, {"EU", day(1), 400}, {"US", day(2), 450},
	}}, nil
}

func TestSemanticSparklineSeriesWindowWhenByIsRowDimension(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "orders"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "day", Type: "time"}},
		Metrics: []sem.Metric{
			{Name: "revenue", Expression: "sum(revenue)"}, {Name: "orders", Expression: "count(*)"},
			{Name: "avg_order", Expression: "{revenue} / nullif({orders}, 0)"},
		}}
	widget := func(by string) *dashboard.Dashboard {
		d := &dashboard.Dashboard{Name: "s", Connection: "warehouse", Model: "sales", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
			Name: "Daily", Type: dashboard.WidgetTypeTable,
			Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}, {Name: "day"}}, MetricRefs: []string{"revenue"},
			Columns: []dashboard.TableColumn{{Name: "trend", Type: "sparkline", Limit: 2,
				X: &dashboard.AxisEncoding{Field: by}, Y: &dashboard.AxisEncoding{Field: "avg_order"}}},
		}}}}}
		d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
		return d
	}

	jobs, err := ResolveWidgetJobs(widget("day"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := jobs[0].Series[0]; s.Window != 1 || s.Keys != 2 || strings.Count(s.SQL, "AS day") != 1 {
		t.Fatalf("series = %+v", s)
	}
	wr := ExecuteWidgetQuery(context.Background(), windowBackend{}, jobs[0])
	if wr.Error != "" {
		t.Fatal(wr.Error)
	}
	day := func(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }
	want := [][]map[string]any{
		{{"day": day(2), "avg_order": 0.91}, {"day": day(3), "avg_order": 1.54}}, // EU Sep 3: last 2 points up to Sep 3
		{{"day": day(1), "avg_order": 1.21}},                                     // EU Sep 1: nothing earlier, a short line
		{{"day": day(1), "avg_order": 1.1}, {"day": day(2), "avg_order": 1.3}},   // US Sep 2: only US points
	}
	for i, row := range wr.Rows {
		if !reflect.DeepEqual(row[3], want[i]) {
			t.Errorf("row %v %v trend = %v, want %v", row[0], row[1], row[3], want[i])
		}
	}

	// Weekly points next to daily rows would return two day columns.
	model.Dimensions[1].Granularities = map[string]string{"week": "date_trunc('week', day)"}
	d := widget("day:week")
	if _, err = ResolveWidgetJobs(d, nil); err == nil || !strings.Contains(err.Error(), "both return column day") {
		t.Fatalf("err = %v", err)
	}
	if err := d.Rows[0].Widgets[0].ValidateSparklineSeries(d); err == nil {
		t.Fatal("validation must catch the clash")
	}
}

func TestSemanticSparklineSeriesSkipsRowFilters(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "orders"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "day", Type: "time"}},
		Metrics:    []sem.Metric{{Name: "revenue", Expression: "sum(revenue)"}}}
	d := &dashboard.Dashboard{Name: "s", Connection: "warehouse", Model: "sales", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
		Name: "Top", Type: dashboard.WidgetTypeTable, GrandTotal: true, Limit: 5,
		Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}}, MetricRefs: []string{"revenue"},
		Filters: []dashboard.SemanticQueryFilter{{Expression: "{revenue} > 100"}, {Expression: "{region} <> 'X'"}},
		Columns: []dashboard.TableColumn{{Name: "trend", Type: "sparkline", X: &dashboard.AxisEncoding{Field: "day"}, Y: &dashboard.AxisEncoding{Field: "revenue"}}},
	}}}}}
	d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
	jobs, err := ResolveWidgetJobs(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	// HAVING and limit pick the shown rows, not points; both queries cover only those rows.
	s := jobs[0].Series[0]
	shown := "HAVING sum(revenue) > 100 LIMIT 5) __grand_total_detail"
	if strings.Count(s.SQL, "HAVING") != 1 || !strings.Contains(s.SQL, shown) || !strings.Contains(s.SQL, "WHERE region <> 'X' GROUP BY 1, 2)") ||
		!strings.Contains(s.TotalSQL, shown) {
		t.Fatalf("series = %+v", s)
	}
}

type mergedBackend struct {
	calls  *atomic.Int32
	result *query.QueryResult // returned on every call, as a cached backend does
}

func (b mergedBackend) Execute(_ context.Context, _ string, sql string) (*query.QueryResult, error) {
	b.calls.Add(1)
	if strings.HasSuffix(sql, " ON TRUE") {
		// The grand total with the total row's points, newest first.
		return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "revenue"}, {Name: "__series_0"}, {Name: "__series_1"}}, Rows: [][]any{
			{1350.0, []any{[]any{"2026-09-03", 300.0}, []any{"2026-09-02", 650.0}}, []any{[]any{"2026-09-03", 9.0}}},
		}}, nil
	}
	return b.result, nil
}

func mergedResult() *query.QueryResult {
	// BigQuery returns each STRUCT(x, y) point as an [x, y] pair, newest first.
	return &query.QueryResult{Columns: []query.ColumnInfo{{Name: "region"}, {Name: "revenue"}, {Name: "__series_0"}, {Name: "__series_1"}}, Rows: [][]any{
		{"EU", 900.0, []any{[]any{"2026-09-03", 300.0}, []any{"2026-09-02", 200.0}, []any{"2026-09-01", 400.0}}, []any{[]any{"2026-09-03", 7.0}}},
		{"US", 450.0, nil, nil},
	}}
}

func TestSemanticSparklineSeriesMergesIntoTableQuery(t *testing.T) {
	model := &sem.Model{Name: "sales", Source: sem.Source{Table: "orders"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "day", Type: "time"}},
		Metrics:    []sem.Metric{{Name: "revenue", Expression: "sum(revenue)"}, {Name: "orders", Expression: "count(*)"}}}
	d := &dashboard.Dashboard{Name: "s", Connection: "warehouse", Model: "sales", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{{
		Name: "Regions", Type: dashboard.WidgetTypeTable, GrandTotal: true,
		Dimensions: []dashboard.SemanticDimensionRef{{Name: "region"}}, MetricRefs: []string{"revenue"},
		Sort: []dashboard.SemanticSort{{Name: "revenue", Direction: "desc"}},
		Columns: []dashboard.TableColumn{
			{Name: "trend", Type: "sparkline", Limit: 2, X: &dashboard.AxisEncoding{Field: "day"}, Y: &dashboard.AxisEncoding{Field: "revenue"}},
			{Name: "orders_trend", Type: "sparkline", X: &dashboard.AxisEncoding{Field: "day"}, Y: &dashboard.AxisEncoding{Field: "orders"}},
		},
	}}}}}
	d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
	job, err := resolveWidgetJob(d, nil, "r0-w0", &d.Rows[0].Widgets[0], func(string) (string, error) { return "bigquery", nil })
	if err != nil {
		t.Fatal(err)
	}
	// Same x: one join; the total row's points join the grand total.
	if len(job.Series) != 1 || job.Series[0].SQL != "" || job.Series[0].TotalSQL != "" || strings.Count(job.SQL, "LEFT JOIN") != 1 ||
		!strings.Contains(job.SQL, "DESC LIMIT 2)") || !strings.HasSuffix(job.SQL, "ORDER BY __rows.revenue DESC") ||
		!strings.HasSuffix(job.GrandTotalSQL, " ON TRUE") {
		t.Fatalf("job = %+v", job)
	}
	var calls atomic.Int32
	backend := mergedBackend{&calls, mergedResult()}
	ExecuteWidgetQuery(context.Background(), backend, *job)
	wr := ExecuteWidgetQuery(context.Background(), backend, *job) // a cached result must still read the same
	if wr.Error != "" || calls.Load() != 4 || wr.Columns[2].Name != "trend" || wr.Columns[3].Name != "orders_trend" {
		t.Fatalf("calls = %d, result = %+v", calls.Load(), wr)
	}
	want := [][]any{
		{[]map[string]any{{"day": "2026-09-02", "revenue": 200.0}, {"day": "2026-09-03", "revenue": 300.0}}, []map[string]any{{"day": "2026-09-03", "orders": 7.0}}},
		{[]map[string]any{}, []map[string]any{}},
	}
	for i, row := range wr.Rows {
		if !reflect.DeepEqual(row[2:], want[i]) {
			t.Errorf("row %v trends = %v, want %v", row[0], row[2:], want[i])
		}
	}
	total := []any{1350.0, []map[string]any{{"day": "2026-09-02", "revenue": 650.0}, {"day": "2026-09-03", "revenue": 300.0}}, []map[string]any{{"day": "2026-09-03", "orders": 9.0}}}
	if gt := wr.GrandTotal; gt == nil || gt.Columns[1].Name != "trend" || !reflect.DeepEqual(gt.Rows[0], total) {
		t.Errorf("grand total = %+v", gt)
	}
}
