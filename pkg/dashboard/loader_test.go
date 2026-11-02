package dashboard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"gopkg.in/yaml.v3"
)

func TestLoadDefinition_SemanticContextAndEncodings(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}},
		Metrics:    []sem.Metric{{Name: "revenue", Expression: "SUM(amount)"}},
	}
	data := []byte(`
name: Sales
model: s
models: {s: sales}
notes:
  - id: rollout
    dimensions: [{name: region, type: select}]
queries:
  totals: {dimensions: [{name: region}], metrics: [revenue]}
rows:
  - widgets:
      - {name: Revenue, type: chart, chart: bar, query: totals, notes: [rollout]}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err)
	assertNoErr(t, Validate(d))
	if d.FilePath != "" || d.Schema == "" {
		t.Fatalf("unexpected metadata: path=%q schema=%q", d.FilePath, d.Schema)
	}
	w := d.Rows[0].Widgets[0]
	if w.XField() != "region" || strings.Join(w.YFields(), ",") != "revenue" {
		t.Fatalf("semantic encodings were not derived: %+v", w)
	}
	_, err = LoadDefinition([]byte(`{"name":"Broken"}`), nil)
	assertErr(t, err)
	withoutModels, err := LoadDefinition(data, nil)
	assertNoErr(t, err)
	assertErr(t, Validate(withoutModels))
}

func TestLoadDefinition_MatchesFileLoading(t *testing.T) {
	for _, path := range []string{"../../testdata/dashboards/sales.yml"} {
		data, err := os.ReadFile(path)
		assertNoErr(t, err)
		file, err := LoadFile(path)
		assertNoErr(t, err)
		memory, err := LoadDefinition(data, nil)
		assertNoErr(t, err)
		if fmt.Sprint(Validate(file)) != fmt.Sprint(Validate(memory)) {
			t.Fatalf("validation differs for %s", path)
		}
	}
}

// ---------------------------------------------------------------------------
// Loader tests
// ---------------------------------------------------------------------------

func TestLoadDir_LoadsAllDashboards(t *testing.T) {
	dashboards, err := LoadDir("../../testdata/dashboards")
	assertNoErr(t, err)

	// 6 YAML + 2 TSX = 8
	if len(dashboards) != 8 {
		t.Fatalf("expected 8 dashboards, got %d", len(dashboards))
	}
}

func TestLoadFile_SalesYAML(t *testing.T) {
	d, err := LoadFile("../../testdata/dashboards/sales.yml")
	assertNoErr(t, err)

	if d.Name != "Sales Analytics" {
		t.Errorf("expected name %q, got %q", "Sales Analytics", d.Name)
	}
	if d.Connection != "local_duckdb" {
		t.Errorf("expected connection %q, got %q", "local_duckdb", d.Connection)
	}
	if len(d.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(d.Filters))
	}
	if len(d.Rows) != 5 {
		t.Errorf("expected 5 rows, got %d", len(d.Rows))
	}
}

func TestLoadFile_ResolvesExternalSQLFiles(t *testing.T) {
	d, err := LoadFile("../../testdata/dashboards/sales.yml")
	assertNoErr(t, err)

	q, ok := d.Queries["revenue_by_month"]
	if !ok {
		t.Fatal("expected query 'revenue_by_month' to exist")
	}
	if q.SQL == "" {
		t.Fatal("expected revenue_by_month SQL to be resolved from file, got empty string")
	}
	if !strings.Contains(q.SQL, "DATE_TRUNC") {
		t.Errorf("expected resolved SQL to contain DATE_TRUNC, got:\n  %s", q.SQL)
	}
}

func TestLoadDir_NonexistentDirectory(t *testing.T) {
	_, err := LoadDir("../../testdata/nonexistent")
	assertErr(t, err)
}

func TestLoadFile_NonexistentFile(t *testing.T) {
	_, err := LoadFile("../../testdata/dashboards/nonexistent.yml")
	assertErr(t, err)
}

func TestLoadFile_DefaultsDashboardSchemaToV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.yml")
	assertNoErr(t, os.WriteFile(path, []byte(`name: Missing Schema
rows:
  - widgets:
      - name: Notes
        type: text
        content: Hello
`), 0o644))

	d, err := LoadFile(path)
	assertNoErr(t, err)
	if d.Schema != "https://getbruin.com/schemas/dac/dashboard/v1" {
		t.Fatalf("expected default schema v1, got %q", d.Schema)
	}
}

func TestLoadFile_KeepsFilterTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.yml")
	assertNoErr(t, os.WriteFile(path, []byte(`name: Tabbed
filters:
  - name: region
    type: text
    tab: Overview
rows:
  - tab: Overview
    widgets:
      - name: Notes
        type: text
        content: Hello
`), 0o644))

	d, err := LoadFile(path)
	assertNoErr(t, err)
	if len(d.Filters) != 1 || d.Filters[0].Tab != "Overview" {
		t.Fatalf("expected filter tab %q, got %+v", "Overview", d.Filters)
	}
}

func TestLoadDir_ProjectRootLoadsDashboardsAndSemanticModels(t *testing.T) {
	dashboards, err := LoadDir("../../testdata/project")
	assertNoErr(t, err)

	if len(dashboards) != 2 {
		t.Fatalf("expected 2 dashboards from project root, got %d", len(dashboards))
	}

	d := FindByName(dashboards, "Semantic Sales")
	if d == nil {
		t.Fatal("expected Semantic Sales dashboard")
	}

	model, modelName, err := d.ResolveSemanticModel("")
	assertNoErr(t, err)
	if model == nil || modelName != "sales" {
		t.Fatalf("expected default model sales, got %q", modelName)
	}
}

func TestLoadFile_ProjectSemanticDashboardAutoSetsXY(t *testing.T) {
	d, err := LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	assertNoErr(t, err)

	trend := d.Rows[1].Widgets[0]
	if trend.XField() != "order_date" {
		t.Fatalf("expected semantic chart x to default to dimension name, got %q", trend.XField())
	}
	if ys := trend.YFields(); len(ys) != 1 || ys[0] != "revenue" {
		t.Fatalf("expected semantic chart y to default to metrics, got %v", ys)
	}

	byCountry := d.Rows[1].Widgets[1]
	if byCountry.XField() != "country" {
		t.Fatalf("expected named semantic query x to default to dimension name, got %q", byCountry.XField())
	}
	if ys := byCountry.YFields(); len(ys) != 1 || ys[0] != "revenue" {
		t.Fatalf("expected named semantic query y to default to metrics, got %v", ys)
	}
}

func TestLoadDir_InvalidSemanticModelsDoNotBlockRegularDashboards(t *testing.T) {
	projectDir := t.TempDir()

	dashboardsDir := filepath.Join(projectDir, "dashboards")
	semanticDir := filepath.Join(projectDir, "semantic")
	assertNoErr(t, os.MkdirAll(dashboardsDir, 0o755))
	assertNoErr(t, os.MkdirAll(semanticDir, 0o755))

	regularDashboard := `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Regular Dashboard
rows:
  - widgets:
      - name: Notes
        type: text
        content: Hello
`
	assertNoErr(t, os.WriteFile(filepath.Join(dashboardsDir, "regular.yml"), []byte(regularDashboard), 0o644))

	invalidModel := `schema: v1
name: broken_sales
source:
  table: sales
dimensions:
  - name: region
    type: string
metrics:
  - name: region
    expression: count(*)
`
	assertNoErr(t, os.WriteFile(filepath.Join(semanticDir, "broken-sales.yml"), []byte(invalidModel), 0o644))

	dashboards, err := LoadDir(projectDir)
	assertNoErr(t, err)

	if len(dashboards) != 1 {
		t.Fatalf("expected 1 dashboard, got %d", len(dashboards))
	}

	if dashboards[0].Name != "Regular Dashboard" {
		t.Fatalf("expected regular dashboard to load, got %q", dashboards[0].Name)
	}
}

func TestLoadDefinition_InheritedModelFormats(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}},
		Metrics: []sem.Metric{
			{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Type: "currency", Currency: "TRY"}},
			{Name: "refunds", Expression: "SUM(refund)", Format: &sem.Format{Type: "currency", Currency: "lira"}},
			{Name: "fees", Expression: "SUM(fee)", Format: &sem.Format{Type: "currency", Currency: "try"}},
		},
	}
	data := []byte(`
name: Sales
model: sales
rows:
  - widgets:
      - {name: Revenue, type: metric, metric: revenue}
      - {name: Refunds, type: metric, metric: refunds}
      - {name: Fees, type: metric, metric: fees}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err) // An invalid model format must not block the dashboard load.
	revenue, ok := d.Rows[0].Widgets[0].Value.Format.(SemanticFormat)
	if !ok || revenue.Currency != "TRY" || revenue.Decimals != nil {
		t.Fatalf("revenue should inherit TRY with Intl default decimals, got %#v", d.Rows[0].Widgets[0].Value.Format)
	}
	if value := d.Rows[0].Widgets[1].Value; value != nil {
		t.Fatalf("invalid model format should not be inherited, got %#v", value)
	}
	if fees, ok := d.Rows[0].Widgets[2].Value.Format.(SemanticFormat); !ok || fees.Currency != "TRY" {
		t.Fatalf("lowercase model currency should be normalized, got %#v", d.Rows[0].Widgets[2].Value.Format)
	}
	// Models that loaded before formats were read must keep validating.
	assertNoErr(t, Validate(d))
}

func TestLoadDefinition_InheritedValueFollowsY(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "channel", Type: "string"}},
		Metrics: []sem.Metric{
			{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Type: "currency", Currency: "TRY"}},
			{Name: "orders", Expression: "COUNT(*)"},
		},
	}
	data := []byte(`
name: Sales
model: sales
rows:
  - widgets:
      - {name: Orders, type: chart, chart: pie, dimension: region, metrics: [revenue, orders], y: {field: orders}}
      - {name: Revenue, type: chart, chart: pie, dimension: region, metrics: [revenue, orders]}
      - {name: Heat, type: chart, chart: heatmap, x: {field: region}, y: {field: channel}, dimensions: [{name: region}, {name: channel}], metrics: [revenue]}
      - {name: Dollars, type: chart, chart: pie, dimension: region, metrics: [revenue], y: {field: revenue, format: "$,.0f"}}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err)
	if value := d.Rows[0].Widgets[0].Value; value != nil {
		t.Fatalf("an explicit y must not be overridden by the first metric, got %#v", value)
	}
	value := d.Rows[0].Widgets[1].Value
	if value == nil || value.Field != "revenue" {
		t.Fatalf("expected revenue value encoding, got %#v", value)
	}
	if format, ok := value.Format.(SemanticFormat); !ok || format.Currency != "TRY" {
		t.Fatalf("revenue should inherit TRY, got %#v", value.Format)
	}
	if heat := d.Rows[0].Widgets[2].Value; heat == nil || heat.Field != "revenue" {
		t.Fatalf("a heatmap's value should come from its metric, not its y dimension, got %#v", heat)
	}
	if dollars := d.Rows[0].Widgets[3].Value; dollars == nil || dollars.Format != "$,.0f" {
		t.Fatalf("an explicit y format must win over the model default, got %#v", dollars)
	}
}

func TestLoadDefinition_InheritedAxisFormats(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}, {Name: "channel", Type: "string"}},
		Metrics: []sem.Metric{
			{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Type: "currency", Currency: "TRY"}},
			{Name: "margin", Expression: "AVG(margin)", Format: &sem.Format{Type: "percentage"}},
		},
	}
	data := []byte(`
name: Sales
model: sales
rows:
  - widgets:
      - name: Partial
        type: chart
        chart: line
        dimension: region
        metrics: [revenue, margin]
        y: {field: [revenue, margin], format: {fields: {margin: {type: percentage, decimals: 1}}}}
      - name: Vega
        type: chart
        chart: vega-lite
        dimensions: [{name: region}, {name: channel}]
        metrics: [revenue]
        spec: {mark: rect, encoding: {color: {field: revenue, type: quantitative}}}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err)
	partial := d.Rows[0].Widgets[0].Y.Format
	if _, revenue := ParseDisplayFormat(partial, "revenue"); revenue == nil || revenue.Currency != "TRY" {
		t.Fatalf("a partial fields map should keep the model format for unlisted fields, got %#v", partial)
	}
	if _, margin := ParseDisplayFormat(partial, "margin"); margin == nil || margin.Decimals == nil || *margin.Decimals != 1 {
		t.Fatalf("an explicit field format must win over the model default, got %#v", partial)
	}
	vega := d.Rows[0].Widgets[1].Y
	if vega == nil {
		t.Fatal("a vega-lite chart with plural dimensions should carry model formats on y")
	}
	if _, revenue := ParseDisplayFormat(vega.Format, "revenue"); revenue == nil || revenue.Currency != "TRY" {
		t.Fatalf("vega-lite revenue should inherit TRY, got %#v", vega.Format)
	}
	assertNoErr(t, Validate(d))
}

func TestDisplayFormatKeepsUnquotedYAMLScalars(t *testing.T) {
	var w struct {
		Y      *AxisEncoding  `yaml:"y"`
		Value  *ValueEncoding `yaml:"value"`
		Column TableColumn    `yaml:"column"`
	}
	assertNoErr(t, yaml.Unmarshal([]byte("y: {field: revenue, format: .2}\nvalue: {field: revenue, format: 05}\ncolumn: {name: revenue, number: +.1}\n"), &w))
	if w.Y.Format != ".2" || w.Value.Format != "05" || w.Column.Number != "+.1" {
		t.Fatalf("unquoted d3 specs should stay strings, got %#v %#v %#v", w.Y.Format, w.Value.Format, w.Column.Number)
	}
}

func TestLoadDefinition_InheritedTableColumnsUseDashboardNames(t *testing.T) {
	models := map[string]*sem.Model{
		"orders": {
			Name: "orders", Source: sem.Source{Table: "orders"},
			Dimensions: []sem.Dimension{{Name: "status", Type: "string"}},
			Metrics:    []sem.Metric{{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Type: "currency", Currency: "EUR"}}},
			Joins:      []sem.Join{{Name: "customers", Relationship: "many_to_one", ForeignKey: "customer_id"}},
		},
		"customers": {
			Name: "customers", Source: sem.Source{Table: "customers"}, PrimaryKey: "customer_id",
			Dimensions: []sem.Dimension{{Name: "region", Type: "string"}},
		},
	}
	data := []byte(`
name: Orders
model: orders
rows:
  - widgets:
      - name: By region
        type: table
        dimensions: [{name: customers.region}]
        metrics: [revenue]
      - name: Uppercase
        type: table
        dimensions: [{name: status}]
        metrics: [revenue]
        columns: [{name: REVENUE}]
`)
	d, err := LoadDefinition(data, models)
	assertNoErr(t, err)
	columns := d.Rows[0].Widgets[0].Columns
	// The server renames the engine's customers_region alias back to the ref.
	if len(columns) != 2 || columns[0].Name != "customers.region" || columns[1].Name != "revenue" {
		t.Fatalf("expected dashboard column names, got %#v", columns)
	}
	if format, ok := columns[1].Number.(SemanticFormat); !ok || format.Currency != "EUR" {
		t.Fatalf("revenue should inherit EUR, got %#v", columns[1].Number)
	}
	if format, ok := d.Rows[0].Widgets[1].Columns[0].Number.(SemanticFormat); !ok || format.Currency != "EUR" {
		t.Fatalf("uppercase warehouse column should inherit EUR, got %#v", d.Rows[0].Widgets[1].Columns[0].Number)
	}
}

func TestTableColumnNormalizesEmptyNumber(t *testing.T) {
	var col TableColumn
	assertNoErr(t, yaml.Unmarshal([]byte("name: region\nnumber: \"\"\n"), &col))
	data, err := json.Marshal(col)
	assertNoErr(t, err)
	if strings.Contains(string(data), "number") {
		t.Fatalf("empty number should be omitted from JSON, got %s", data)
	}
	out, err := yaml.Marshal(col)
	assertNoErr(t, err)
	if strings.Contains(string(out), "number") {
		t.Fatalf("empty number should be omitted from YAML, got %s", out)
	}
}

func TestLoadDefinition_CurrencyShorthandNeedsModelCode(t *testing.T) {
	models := map[string]*sem.Model{
		"orders": {
			Name: "orders", Source: sem.Source{Table: "orders"},
			Dimensions: []sem.Dimension{{Name: "status", Type: "string"}},
			Metrics: []sem.Metric{
				{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Decimals: 2}},
				{Name: "profit", Expression: "SUM(profit)", Format: &sem.Format{Type: "currency", Currency: "EUR"}},
			},
		},
	}
	data := []byte(`
name: Orders
model: orders
rows:
  - widgets:
      - name: Table
        type: table
        dimensions: [{name: status}]
        metrics: [revenue, profit]
        columns: [{name: revenue, number: currency}, {name: profit, number: currency}]
`)
	d, err := LoadDefinition(data, models)
	assertNoErr(t, err)
	columns := d.Rows[0].Widgets[0].Columns
	if columns[0].Number != "currency" {
		t.Fatalf("a model without a currency code should keep the $ shorthand, got %#v", columns[0].Number)
	}
	if format, ok := columns[1].Number.(SemanticFormat); !ok || format.Currency != "EUR" || format.Type != "currency" {
		t.Fatalf("a model currency code should upgrade the shorthand, got %#v", columns[1].Number)
	}
}

func TestValidateDisplayFormatCurrencyCodes(t *testing.T) {
	for _, code := range []string{"TRY", "VES", "MRU"} {
		if err := validateDisplayFormat(map[string]any{"currency": code}); err != nil {
			t.Errorf("%s should be accepted: %v", code, err)
		}
	}
	for _, code := range []string{"try", "US$", "EURO"} {
		if err := validateDisplayFormat(map[string]any{"currency": code}); err == nil {
			t.Errorf("%s should be rejected", code)
		}
	}
}

// An inherited value must not satisfy "value is required": otherwise validation
// passes only when the model has formats.
func TestValidate_InheritedValueDoesNotSatisfyRequiredValue(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}},
		Metrics:    []sem.Metric{{Name: "revenue", Expression: "SUM(amount)", Format: &sem.Format{Type: "currency", Currency: "TRY"}}},
	}
	data := []byte(`
name: Sales
model: sales
queries:
  total: {metrics: [revenue]}
rows:
  - widgets:
      - {name: Gauge, type: chart, chart: gauge, query: total}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err)
	if d.Rows[0].Widgets[0].Value == nil {
		t.Fatal("the gauge should still carry the inherited format")
	}
	if err := Validate(d); err == nil || !strings.Contains(err.Error(), "value is required for gauge charts") {
		t.Fatalf("expected the authored value to be required, got %v", err)
	}
}

// An explicit y format covers the value it supplies even when no model metric has a format.
func TestLoadDefinition_ExplicitYFormatWithoutModelFormats(t *testing.T) {
	model := &sem.Model{
		Name: "sales", Source: sem.Source{Table: "sales"},
		Dimensions: []sem.Dimension{{Name: "region", Type: "string"}},
		Metrics:    []sem.Metric{{Name: "revenue", Expression: "SUM(amount)"}},
	}
	data := []byte(`
name: Sales
model: sales
rows:
  - widgets:
      - {name: Dollars, type: chart, chart: pie, dimension: region, metrics: [revenue], y: {field: revenue, format: "$,.0f"}}
      - {name: Plain, type: chart, chart: pie, dimension: region, metrics: [revenue]}
`)
	d, err := LoadDefinition(data, map[string]*sem.Model{"sales": model})
	assertNoErr(t, err)
	if value := d.Rows[0].Widgets[0].Value; value == nil || value.Format != "$,.0f" {
		t.Fatalf("the y format should cover the pie value, got %#v", value)
	}
	if value := d.Rows[0].Widgets[1].Value; value != nil {
		t.Fatalf("an unformatted widget should load unchanged, got %#v", value)
	}
	assertNoErr(t, Validate(d))
}
