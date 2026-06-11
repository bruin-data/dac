package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TSX loader tests
// ---------------------------------------------------------------------------

func TestLoadTSXFile_DataDrivenDashboard(t *testing.T) {
	// The sample dashboard uses query() at load time. Without a backend,
	// query() returns empty results, so the data-driven loops produce no widgets.
	// This tests that the file loads without error even without a backend.
	d, err := LoadTSXFile("../../testdata/dashboards/simple.dashboard.tsx")
	assertNoErr(t, err)

	if d.Name != "Sales (TSX)" {
		t.Errorf("expected name %q, got %q", "Sales (TSX)", d.Name)
	}
	if d.Connection != "local_duckdb" {
		t.Errorf("expected connection %q, got %q", "local_duckdb", d.Connection)
	}
	if len(d.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(d.Filters))
	}
}

func TestLoadTSXFile_DataDrivenWithMockBackend(t *testing.T) {
	// Provide a mock query backend that returns realistic results.
	mockQuery := func(conn, sql string) (map[string]interface{}, error) {
		switch {
		case strings.Contains(sql, "DISTINCT region"):
			return map[string]interface{}{
				"columns": []interface{}{map[string]interface{}{"name": "region"}},
				"rows":    []interface{}{[]interface{}{"APAC"}, []interface{}{"Europe"}, []interface{}{"North America"}},
			}, nil
		case strings.Contains(sql, "DISTINCT status"):
			return map[string]interface{}{
				"columns": []interface{}{map[string]interface{}{"name": "status"}},
				"rows":    []interface{}{[]interface{}{"completed"}, []interface{}{"pending"}, []interface{}{"shipped"}},
			}, nil
		case strings.Contains(sql, "information_schema.tables"):
			return map[string]interface{}{
				"columns": []interface{}{map[string]interface{}{"name": "table_name"}},
				"rows":    []interface{}{[]interface{}{"orders"}, []interface{}{"sales"}},
			}, nil
		default:
			return map[string]interface{}{"columns": []interface{}{}, "rows": []interface{}{}}, nil
		}
	}

	d, err := LoadTSXFile("../../testdata/dashboards/simple.dashboard.tsx", WithQueryFunc(mockQuery))
	assertNoErr(t, err)

	if d.Name != "Sales (TSX)" {
		t.Errorf("expected name %q, got %q", "Sales (TSX)", d.Name)
	}

	// Row 0: 3 region KPIs (auto-discovered)
	if len(d.Rows[0].Widgets) != 3 {
		t.Fatalf("expected 3 region widgets, got %d", len(d.Rows[0].Widgets))
	}
	if d.Rows[0].Widgets[0].Name != "APAC" {
		t.Errorf("expected first region %q, got %q", "APAC", d.Rows[0].Widgets[0].Name)
	}

	// Row 1: 3 status KPIs (auto-discovered)
	if len(d.Rows[1].Widgets) != 3 {
		t.Fatalf("expected 3 status widgets, got %d", len(d.Rows[1].Widgets))
	}
	if d.Rows[1].Widgets[0].Name != "Completed Orders" {
		t.Errorf("expected %q, got %q", "Completed Orders", d.Rows[1].Widgets[0].Name)
	}

	// Row 2: charts (stacked bar split by color + pie)
	if len(d.Rows[2].Widgets) != 2 {
		t.Fatalf("expected 2 chart widgets, got %d", len(d.Rows[2].Widgets))
	}
	bar := d.Rows[2].Widgets[0]
	if bar.Chart != "bar" {
		t.Errorf("expected bar chart, got %q", bar.Chart)
	}
	if got := bar.YFields(); len(got) != 1 || got[0] != "revenue" {
		t.Errorf("expected y = [revenue], got %v", got)
	}
	if !bar.Stacked {
		t.Error("expected stacked bar")
	}
	if bar.ColorField() != "region" {
		t.Errorf("expected color field %q, got %q", "region", bar.ColorField())
	}

	// Rows 3-4: auto-discovered table previews
	if len(d.Rows) != 5 {
		t.Fatalf("expected 5 rows total, got %d", len(d.Rows))
	}
	if d.Rows[3].Widgets[0].Name != "orders" {
		t.Errorf("expected table %q, got %q", "orders", d.Rows[3].Widgets[0].Name)
	}
	if d.Rows[4].Widgets[0].Name != "sales" {
		t.Errorf("expected table %q, got %q", "sales", d.Rows[4].Widgets[0].Name)
	}

	// Filter options should include data-driven values
	regionFilter := d.Filters[1] // region is second filter
	if regionFilter.Options == nil {
		t.Fatal("expected region filter to have options")
	}
	// Should be ["All", "APAC", "Europe", "North America"]
	if len(regionFilter.Options.Values) != 4 {
		t.Errorf("expected 4 filter values, got %d: %v", len(regionFilter.Options.Values), regionFilter.Options.Values)
	}
}

func TestEvalTSX_LoopsAndVariables(t *testing.T) {
	source := `
const regions = ["NA", "EU", "APAC"]

export default (
  <Dashboard name="Loop Test" connection="duckdb">
    <Row>
      {regions.map(r =>
        <Metric name={r + " Revenue"} col={4}
          sql={"SELECT SUM(amount) as value FROM sales WHERE region = '" + r + "'"}
          value={{ field: "value", type: "number", format: "$,.0f" }} />
      )}
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	if d.Name != "Loop Test" {
		t.Errorf("expected name %q, got %q", "Loop Test", d.Name)
	}
	if len(d.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(d.Rows))
	}
	if len(d.Rows[0].Widgets) != 3 {
		t.Fatalf("expected 3 widgets from map(), got %d", len(d.Rows[0].Widgets))
	}

	names := []string{"NA Revenue", "EU Revenue", "APAC Revenue"}
	for i, w := range d.Rows[0].Widgets {
		if w.Name != names[i] {
			t.Errorf("widget %d: expected name %q, got %q", i, names[i], w.Name)
		}
		if w.Value == nil || w.Value.Field != "value" {
			t.Errorf("widget %d: expected value field %q, got %+v", i, "value", w.Value)
		}
		if w.Value == nil || w.Value.Format != "$,.0f" {
			t.Errorf("widget %d: expected format %q, got %+v", i, "$,.0f", w.Value)
		}
		if !strings.Contains(w.SQL, "WHERE region") {
			t.Errorf("widget %d: expected SQL with region filter", i)
		}
	}
}

func TestEvalTSX_CustomComponent(t *testing.T) {
	source := `
function KPI({ name, sql, format = ",.0f", ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format }} {...rest} />
}

export default (
  <Dashboard name="Custom Component" connection="duckdb">
    <Row>
      <KPI name="Revenue" sql="SELECT 100 as value" format="$,.0f" col={4} />
      <KPI name="Orders" sql="SELECT 42 as value" col={4} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	if len(d.Rows[0].Widgets) != 2 {
		t.Fatalf("expected 2 widgets, got %d", len(d.Rows[0].Widgets))
	}

	w0 := d.Rows[0].Widgets[0]
	if w0.Name != "Revenue" {
		t.Errorf("expected %q, got %q", "Revenue", w0.Name)
	}
	if w0.Type != WidgetTypeMetric {
		t.Errorf("expected metric, got %q", w0.Type)
	}
	if w0.Value == nil || w0.Value.Field != "value" {
		t.Errorf("expected value field %q, got %+v", "value", w0.Value)
	}
	if w0.Value == nil || w0.Value.Format != "$,.0f" {
		t.Errorf("expected format %q, got %+v", "$,.0f", w0.Value)
	}
}

func TestEvalTSX_JinjaTemplatesPreserved(t *testing.T) {
	source := `
export default (
  <Dashboard name="Jinja Test" connection="db">
    <Row>
      <Chart name="Revenue" chart="line" col={12}
        sql={"SELECT month, SUM(amount) as rev FROM sales WHERE region = '{{ filters.region }}' GROUP BY 1"}
        x="month" y={["rev"]} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	w := d.Rows[0].Widgets[0]
	if !strings.Contains(w.SQL, "{{ filters.region }}") {
		t.Errorf("Jinja template markers should be preserved, got: %s", w.SQL)
	}
}

func TestEvalTSX_SemanticLayer(t *testing.T) {
	source := `
export default (
  <Dashboard name="Semantic Test" connection="gcp">
    <Semantic
      source={{ table: "events", dateColumn: "event_date", dateFormat: "%Y%m%d" }}
      metrics={{
        page_views: { aggregate: "count", filter: { event_name: "page_view" } },
        users: { aggregate: "count_distinct", column: "user_id" },
      }}
      dimensions={{
        daily: { column: "event_date", type: "date" },
        country: { column: "geo.country" },
      }}
    />
    <Row>
      <Metric name="Page Views" metric="page_views" col={3} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	if d.Semantic == nil {
		t.Fatal("expected semantic layer")
	}
	if d.Semantic.Source == nil {
		t.Fatal("expected semantic source")
	}
	if d.Semantic.Source.Table != "events" {
		t.Errorf("expected table %q, got %q", "events", d.Semantic.Source.Table)
	}
	if d.Semantic.Source.DateColumn != "event_date" {
		t.Errorf("expected dateColumn %q, got %q", "event_date", d.Semantic.Source.DateColumn)
	}
	if d.Semantic.Source.DateFormat != "%Y%m%d" {
		t.Errorf("expected dateFormat %q, got %q", "%Y%m%d", d.Semantic.Source.DateFormat)
	}

	if len(d.Semantic.Metrics) != 2 {
		t.Fatalf("expected 2 metrics, got %d", len(d.Semantic.Metrics))
	}
	pv := d.Semantic.Metrics["page_views"]
	if pv.Aggregate != "count" {
		t.Errorf("expected count aggregate, got %q", pv.Aggregate)
	}
	if pv.Filter["event_name"] != "page_view" {
		t.Errorf("expected filter event_name=page_view, got %v", pv.Filter)
	}

	users := d.Semantic.Metrics["users"]
	if users.Aggregate != "count_distinct" {
		t.Errorf("expected count_distinct aggregate, got %q", users.Aggregate)
	}
	if users.Column != "user_id" {
		t.Errorf("expected column %q, got %q", "user_id", users.Column)
	}

	if len(d.Semantic.Dimensions) != 2 {
		t.Fatalf("expected 2 dimensions, got %d", len(d.Semantic.Dimensions))
	}
	daily := d.Semantic.Dimensions["daily"]
	if daily.Type != "date" {
		t.Errorf("expected date type, got %q", daily.Type)
	}
	country := d.Semantic.Dimensions["country"]
	if country.Column != "geo.country" {
		t.Errorf("expected column %q, got %q", "geo.country", country.Column)
	}
}

func TestEvalTSX_QueryAtLoadTime(t *testing.T) {
	source := `
const result = query("duckdb", "SELECT table_name FROM information_schema.tables")

export default (
  <Dashboard name="Dynamic" connection="duckdb">
    {result.rows.map(function(row) {
      return (
        <Row>
          <Table name={"Preview: " + row[0]} col={12}
            sql={"SELECT * FROM " + row[0] + " LIMIT 10"} />
        </Row>
      )
    })}
  </Dashboard>
)
`
	mockQuery := func(connection, sql string) (map[string]interface{}, error) {
		return map[string]interface{}{
			"columns": []map[string]interface{}{{"name": "table_name"}},
			"rows":    []interface{}{[]interface{}{"users"}, []interface{}{"orders"}, []interface{}{"products"}},
		}, nil
	}

	d, err := evalTSX(source, "test.tsx", &tsxConfig{queryFn: mockQuery})
	assertNoErr(t, err)

	if d.Name != "Dynamic" {
		t.Errorf("expected %q, got %q", "Dynamic", d.Name)
	}
	if len(d.Rows) != 3 {
		t.Fatalf("expected 3 rows from query, got %d", len(d.Rows))
	}

	expectedNames := []string{"Preview: users", "Preview: orders", "Preview: products"}
	for i, row := range d.Rows {
		if len(row.Widgets) != 1 {
			t.Errorf("row %d: expected 1 widget, got %d", i, len(row.Widgets))
			continue
		}
		if row.Widgets[0].Name != expectedNames[i] {
			t.Errorf("row %d: expected name %q, got %q", i, expectedNames[i], row.Widgets[0].Name)
		}
	}
}

func TestEvalTSX_NamedQuery(t *testing.T) {
	source := `
export default (
  <Dashboard name="Named Query" connection="db">
    <Query name="total_revenue" sql="SELECT SUM(amount) as value FROM sales" />
    <Row>
      <Metric name="Revenue" query="total_revenue" value="value" col={6} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	if len(d.Queries) != 1 {
		t.Fatalf("expected 1 named query, got %d", len(d.Queries))
	}
	q, ok := d.Queries["total_revenue"]
	if !ok {
		t.Fatal("expected query 'total_revenue'")
	}
	if q.SQL != "SELECT SUM(amount) as value FROM sales" {
		t.Errorf("unexpected SQL: %s", q.SQL)
	}

	w := d.Rows[0].Widgets[0]
	if w.QueryRef != "total_revenue" {
		t.Errorf("expected query ref %q, got %q", "total_revenue", w.QueryRef)
	}
}

func TestEvalTSX_TextAndDividerWidgets(t *testing.T) {
	source := `
export default (
  <Dashboard name="Text Test" connection="db">
    <Row>
      <Text name="Note" content="Hello **world**" col={6} />
      <Divider name="sep" col={6} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	w0 := d.Rows[0].Widgets[0]
	if w0.Type != WidgetTypeText {
		t.Errorf("expected text, got %q", w0.Type)
	}
	if w0.Content != "Hello **world**" {
		t.Errorf("expected content %q, got %q", "Hello **world**", w0.Content)
	}

	w1 := d.Rows[0].Widgets[1]
	if w1.Type != WidgetTypeDivider {
		t.Errorf("expected divider, got %q", w1.Type)
	}
}

func TestEvalTSX_ImageWidget(t *testing.T) {
	source := `
export default (
  <Dashboard name="Image Test" connection="db">
    <Row>
      <Image name="Logo" src="https://example.com/logo.png" alt="Company Logo" col={12} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	w := d.Rows[0].Widgets[0]
	if w.Type != WidgetTypeImage {
		t.Errorf("expected image, got %q", w.Type)
	}
	if w.Src != "https://example.com/logo.png" {
		t.Errorf("expected src %q, got %q", "https://example.com/logo.png", w.Src)
	}
	if w.Alt != "Company Logo" {
		t.Errorf("expected alt %q, got %q", "Company Logo", w.Alt)
	}
}

func TestEvalTSX_TableWithColumns(t *testing.T) {
	source := `
export default (
  <Dashboard name="Table Cols" connection="db">
    <Row>
      <Table name="Orders" col={12}
        sql="SELECT * FROM orders"
        columns={[
          { name: "id", label: "Order ID" },
          { name: "amount", label: "Amount", format: "currency" },
        ]} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	w := d.Rows[0].Widgets[0]
	if len(w.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(w.Columns))
	}
	if w.Columns[0].Name != "id" || w.Columns[0].Label != "Order ID" {
		t.Errorf("unexpected column 0: %+v", w.Columns[0])
	}
	if w.Columns[1].Format != "currency" {
		t.Errorf("expected format %q, got %q", "currency", w.Columns[1].Format)
	}
}

func TestEvalTSX_StackedBarChart(t *testing.T) {
	source := `
export default (
  <Dashboard name="Stacked" connection="db">
    <Row>
      <Chart name="Sales" chart="bar" stacked={true} col={12}
        sql="SELECT month, na, eu FROM sales"
        x="month" y={["na", "eu"]} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	w := d.Rows[0].Widgets[0]
	if !w.Stacked {
		t.Error("expected stacked=true")
	}
	if len(w.YFields()) != 2 {
		t.Errorf("expected 2 y series, got %d", len(w.YFields()))
	}
}

func TestEvalTSX_ErrorNoDefaultExport(t *testing.T) {
	source := `const x = 1`
	_, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertErr(t, err)
}

func TestEvalTSX_ErrorNotDashboardRoot(t *testing.T) {
	source := `export default <Row><Metric name="x" sql="SELECT 1" col={12} /></Row>`
	_, err := evalTSX(source, "test.tsx", &tsxConfig{})
	if err == nil {
		t.Fatal("expected error for non-Dashboard root")
	}
	if !strings.Contains(err.Error(), "root element must be <Dashboard>") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTranspileTSX_Basic(t *testing.T) {
	source := `const x = <Metric name="test" col={3} />`
	js, err := transpileTSX(source)
	assertNoErr(t, err)

	// esbuild converts capitalized JSX tags to variable references: h(Metric, ...)
	if !strings.Contains(js, "h(Metric") {
		t.Errorf("expected h() call in output, got:\n%s", js)
	}
}

func TestTranspileTSX_Error(t *testing.T) {
	source := `const x = <Metric name="test" col={}`
	_, err := transpileTSX(source)
	assertErr(t, err)
}

func TestIsTSXFile(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"sales.dashboard.tsx", true},
		{"test.dashboard.tsx", true},
		{"helper.tsx", false},
		{"dashboard.tsx", false},
		{"sales.yml", false},
		{"lib/kpi.tsx", false},
	}
	for _, tt := range tests {
		if got := IsTSXFile(tt.name); got != tt.want {
			t.Errorf("IsTSXFile(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// LoadDir with mixed YAML + TSX
// ---------------------------------------------------------------------------

func TestLoadDir_MixedYAMLAndTSX(t *testing.T) {
	dashboards, err := LoadDir("../../testdata/dashboards")
	assertNoErr(t, err)

	// Should find 6 YAML + 2 TSX = 8 dashboards.
	if len(dashboards) != 8 {
		names := make([]string, len(dashboards))
		for i, d := range dashboards {
			names[i] = d.Name
		}
		t.Fatalf("expected 8 dashboards, got %d: %v", len(dashboards), names)
	}

	// Verify TSX dashboard was loaded.
	found := false
	for _, d := range dashboards {
		if d.Name == "Sales (TSX)" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find 'Sales (TSX)' dashboard from .dashboard.tsx file")
	}
}

// ---------------------------------------------------------------------------
// Require/module system
// ---------------------------------------------------------------------------

func TestEvalTSX_RequireModule(t *testing.T) {
	// Create a temp dir with a dashboard and a lib module.
	tmpDir := t.TempDir()

	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write the shared module.
	helperCode := `
function KPI({ name, sql }) {
  return h("Metric", { name: name, sql: sql, value: { field: "value", type: "number", format: ",.0f" } })
}
module.exports = { KPI }
`
	if err := os.WriteFile(filepath.Join(libDir, "kpi.js"), []byte(helperCode), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write the dashboard.
	dashCode := `
const { KPI } = require("./lib/kpi.js")

export default (
  <Dashboard name="Require Test" connection="db">
    <Row>
      <KPI name="Revenue" sql="SELECT 100 as value" />
    </Row>
  </Dashboard>
)
`
	dashPath := filepath.Join(tmpDir, "test.dashboard.tsx")
	if err := os.WriteFile(dashPath, []byte(dashCode), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := LoadTSXFile(dashPath)
	assertNoErr(t, err)

	if d.Name != "Require Test" {
		t.Errorf("expected %q, got %q", "Require Test", d.Name)
	}
	if len(d.Rows) != 1 || len(d.Rows[0].Widgets) != 1 {
		t.Fatalf("expected 1 row with 1 widget, got %d rows", len(d.Rows))
	}
	w := d.Rows[0].Widgets[0]
	if w.Name != "Revenue" {
		t.Errorf("expected name %q, got %q", "Revenue", w.Name)
	}
	if w.Type != WidgetTypeMetric {
		t.Errorf("expected metric, got %q", w.Type)
	}
}

func TestEvalTSX_RequireTSXModule(t *testing.T) {
	tmpDir := t.TempDir()

	libDir := filepath.Join(tmpDir, "lib")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a .tsx helper module.
	helperCode := `
function KPI({ name, sql, ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format: ",.0f" }} {...rest} />
}
module.exports = { KPI }
`
	if err := os.WriteFile(filepath.Join(libDir, "kpi.tsx"), []byte(helperCode), 0o644); err != nil {
		t.Fatal(err)
	}

	dashCode := `
const { KPI } = require("./lib/kpi")

export default (
  <Dashboard name="TSX Require" connection="db">
    <Row>
      <KPI name="Orders" sql="SELECT 42 as value" col={6} />
    </Row>
  </Dashboard>
)
`
	dashPath := filepath.Join(tmpDir, "test.dashboard.tsx")
	if err := os.WriteFile(dashPath, []byte(dashCode), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := LoadTSXFile(dashPath)
	assertNoErr(t, err)

	if d.Name != "TSX Require" {
		t.Errorf("expected %q, got %q", "TSX Require", d.Name)
	}
	w := d.Rows[0].Widgets[0]
	if w.Name != "Orders" {
		t.Errorf("expected %q, got %q", "Orders", w.Name)
	}
	if w.Col != 6 {
		t.Errorf("expected col 6, got %d", w.Col)
	}
}

func TestEvalTSX_RequireJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a JSON config file.
	jsonContent := `{"regions": ["NA", "EU", "APAC"]}`
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	dashCode := `
const config = require("./config.json")

export default (
  <Dashboard name="JSON Require" connection="db">
    <Row>
      {config.regions.map(function(r) {
        return <Metric name={r} sql={"SELECT 1 as value"} value="value" col={4} />
      })}
    </Row>
  </Dashboard>
)
`
	dashPath := filepath.Join(tmpDir, "test.dashboard.tsx")
	if err := os.WriteFile(dashPath, []byte(dashCode), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := LoadTSXFile(dashPath)
	assertNoErr(t, err)

	if len(d.Rows[0].Widgets) != 3 {
		t.Fatalf("expected 3 widgets, got %d", len(d.Rows[0].Widgets))
	}
}

// ---------------------------------------------------------------------------
// Include SQL files
// ---------------------------------------------------------------------------

func TestEvalTSX_IncludeSQLFile(t *testing.T) {
	tmpDir := t.TempDir()

	sqlDir := filepath.Join(tmpDir, "queries")
	if err := os.MkdirAll(sqlDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sqlContent := "SELECT * FROM orders ORDER BY created_at DESC LIMIT 20"
	if err := os.WriteFile(filepath.Join(sqlDir, "recent.sql"), []byte(sqlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	dashCode := `
const sql = include("queries/recent.sql")

export default (
  <Dashboard name="Include Test" connection="db">
    <Row>
      <Table name="Recent" sql={sql} col={12} />
    </Row>
  </Dashboard>
)
`
	dashPath := filepath.Join(tmpDir, "test.dashboard.tsx")
	if err := os.WriteFile(dashPath, []byte(dashCode), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := LoadTSXFile(dashPath)
	assertNoErr(t, err)

	w := d.Rows[0].Widgets[0]
	if w.SQL != sqlContent {
		t.Errorf("expected SQL from file, got: %s", w.SQL)
	}
}

func TestLoadTSXFile_ProjectSemanticDashboard(t *testing.T) {
	d, err := LoadTSXFile("../../testdata/project/dashboards/semantic.dashboard.tsx")
	assertNoErr(t, err)

	if d.Model != "sales" {
		t.Fatalf("expected dashboard model sales, got %q", d.Model)
	}
	if d.Models["sales_model"] != "sales" {
		t.Fatalf("expected alias sales_model -> sales, got %v", d.Models)
	}

	trend := d.Rows[1].Widgets[0]
	if trend.Granularity != "month" {
		t.Fatalf("expected month granularity, got %q", trend.Granularity)
	}
	if trend.XField() != "order_date" {
		t.Fatalf("expected trend x order_date, got %q", trend.XField())
	}

	table := d.Rows[2].Widgets[0]
	if table.Model != "sales_model" {
		t.Fatalf("expected table model alias, got %q", table.Model)
	}
	if len(table.Dimensions) != 1 || table.Dimensions[0].Name != "country" {
		t.Fatalf("expected semantic table dimensions, got %+v", table.Dimensions)
	}
}
