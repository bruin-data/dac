package schemas

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchemasValidateMinimalDocuments(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Minimal
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "dashboard filter scoped to a tab",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Tabbed
filters:
  - name: region
    type: text
    tab: Overview
rows:
  - tab: Overview
    widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "dashboard notes",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Notes
notes:
  - id: note1
    dimensions:
      - { name: app, type: text, required: true }
      - { name: country, type: text, multiselect: true }
      - { name: priority, type: number }
      - { name: city, type: select, multiselect: true }
      - { name: segment, type: select }
      - { name: release, type: date }
      - { name: active, type: boolean }
      - { name: comment, type: text }
      - { name: legacy, multiselect: true }
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
        notes: [note1]
`,
		},
		{
			name:     "dashboard note with no scoping dimensions",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: dashboard_context
    dimensions: []
rows:
  - widgets:
      - name: Context
        type: text
        content: Dashboard context
        notes: [dashboard_context]
`,
		},
		{
			name:     "dashboard filter types",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Filter Types
filters:
  - name: as_of_date
    type: date
    default: "2025-01-31"
  - name: min_value
    type: number
    default: 100
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "dashboard vega-lite chart",
			schemaID: DashboardV1ID,
			yaml: `name: Vega-Lite
rows:
  - widgets:
      - name: Layered
        type: chart
        chart: vega-lite
        data:
          columns: [x, y]
          rows: [[A, 1], [B, 2]]
        spec:
          data: { name: dac }
          layer:
            - mark: line
            - mark: point
`,
		},
		{
			name:     "dashboard pivot table",
			schemaID: DashboardV1ID,
			yaml: `name: Pivot
rows:
  - widgets:
      - name: Sales
        type: pivot_table
        sql: SELECT region, product, sales FROM orders
        pivot:
          rows:
            - { field: region, order: asc, showTotals: true }
          columns:
            - { field: product }
          values:
            - field: sales
              summarize: sum
              label: Total Sales
              format:
                - { backgroundColor: ["#FECACA", "#BBF7D0"], range: [0, 100], unit: absolute, scaleBy: row }
`,
		},
		{
			name:     "dashboard image column",
			schemaID: DashboardV1ID,
			yaml: `name: Listings
rows:
  - widgets:
      - name: Active
        type: table
        sql: SELECT photo, price FROM listings
        columns:
          - { name: photo, type: image }
          - { name: price, number: currency }
`,
		},
		{
			name:     "dashboard sparkline column",
			schemaID: DashboardV1ID,
			yaml: `name: Accounts
rows:
  - widgets:
      - name: Active
        type: table
        sql: SELECT account, trend FROM accounts
        columns:
          - { name: account }
          - name: trend
            type: sparkline
            x: { field: day, type: date, format: "%b %d" }
            y: { field: amount, type: number, format: "$,.0f", beginAtZero: true }
`,
		},
		{
			name:     "dashboard sparkline column with legacy scalar format",
			schemaID: DashboardV1ID,
			yaml: `name: Accounts
rows:
  - widgets:
      - name: Active
        type: table
        sql: SELECT trend FROM accounts
        columns:
          - { name: trend, type: sparkline, x: { field: day }, y: { field: amount }, format: "$,.0f" }
`,
		},
		{
			name:     "dashboard widget tabs",
			schemaID: DashboardV1ID,
			yaml: `name: Widget Tabs
rows:
  - widgets:
      - name: Sales
        type: tabs
        tabs:
          - name: Revenue
            type: chart
            chart: bar
            data:
              columns: [month, revenue]
              rows: [[Jan, 1]]
            x: { field: month, type: category }
            y: { field: revenue, type: number }
          - name: Details
            type: table
            data:
              columns: [month, orders]
              rows: [[Jan, 1]]
`,
		},
		{
			name:     "dashboard tabbed pivot_table",
			schemaID: DashboardV1ID,
			yaml: `name: Tabbed Pivot
rows:
  - widgets:
      - name: Cohorts
        type: tabs
        tabs:
          - name: A
            type: pivot_table
            sql: SELECT 1 AS r, 2 AS v
            pivot:
              rows: [{ field: r }]
              values: [{ field: v }]
`,
		},
		{
			name:     "theme",
			schemaID: ThemeV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/theme/v1
name: corporate
tokens:
  background: "#ffffff"
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err != nil {
				t.Fatalf("expected valid %s: %v", tt.name, err)
			}
		})
	}
}

func TestSchemasAllowMissingSchema(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard",
			schemaID: DashboardV1ID,
			yaml: `name: Minimal
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "theme",
			schemaID: ThemeV1ID,
			yaml: `name: corporate
tokens:
  background: "#ffffff"
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err != nil {
				t.Fatalf("expected valid %s without schema: %v", tt.name, err)
			}
		})
	}
}

func TestSchemasRejectInvalidDocuments(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard note without dimensions",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: dashboard_context
rows:
  - widgets:
      - name: Context
        type: text
        content: Dashboard context
`,
		},
		{
			name:     "dashboard note with empty dimension type",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: app, type: "" }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard note with invalid dimension type",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: region, type: dropdown }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard note with unsupported options",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: priority, type: select, options: { values: [p0, p1] } }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard note with unsupported default",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: city, type: select, default: London }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard note with multiselect boolean",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: active, type: boolean, multiselect: true }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard note with unsupported multiple",
			schemaID: DashboardV1ID,
			yaml: `name: Notes
notes:
  - id: invalid
    dimensions:
      - { name: region, type: text, multiple: true }
rows:
  - widgets:
      - { name: Context, type: text, content: Dashboard context }
`,
		},
		{
			name:     "dashboard filter with an empty tab",
			schemaID: DashboardV1ID,
			yaml: `name: Empty Tab
filters:
  - { name: region, type: text, tab: "" }
rows:
  - widgets:
      - name: One
        type: metric
`,
		},
		{
			name:     "dashboard filter with a non-string tab",
			schemaID: DashboardV1ID,
			yaml: `name: Numeric Tab
filters:
  - { name: region, type: text, tab: 3 }
rows:
  - widgets:
      - name: One
        type: metric
`,
		},
		{
			name:     "dashboard wrong schema",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v2
name: Wrong Schema
rows:
  - widgets:
      - name: One
        type: metric
`,
		},
		{
			name:     "theme missing tokens",
			schemaID: ThemeV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/theme/v1
name: corporate
`,
		},
		{
			name:     "nested widget tabs",
			schemaID: DashboardV1ID,
			yaml: `name: Nested Tabs
rows:
  - widgets:
      - name: Sales
        type: tabs
        tabs:
          - name: Revenue
            type: table
            sql: SELECT 1
            tabs:
              - name: Inner
                sql: SELECT 2
`,
		},
		{
			name:     "pivot on non-pivot_table widget",
			schemaID: DashboardV1ID,
			yaml: `name: Pivot On Table
rows:
  - widgets:
      - name: Sales
        type: table
        sql: SELECT region, sales FROM orders
        pivot:
          rows:
            - { field: region }
          values:
            - { field: sales }
`,
		},
		{
			name:     "pivot_table widget without pivot",
			schemaID: DashboardV1ID,
			yaml: `name: Pivot Missing
rows:
  - widgets:
      - name: Sales
        type: pivot_table
        sql: SELECT region, sales FROM orders
`,
		},
		{
			name:     "image column on pivot_table widget",
			schemaID: DashboardV1ID,
			yaml: `name: Pivot Image
rows:
  - widgets:
      - name: Sales
        type: pivot_table
        sql: SELECT region, photo FROM listings
        pivot:
          rows:
            - { field: region }
          values:
            - { field: photo }
        columns:
          - { name: photo, type: image }
`,
		},
		{
			name:     "scaleBy on a table column format",
			schemaID: DashboardV1ID,
			yaml: `name: ScaleBy On Table
rows:
  - widgets:
      - name: Sales
        type: table
        sql: SELECT sales FROM orders
        columns:
          - name: sales
            format:
              - { backgroundColor: ["red", "green"], scaleBy: row }
`,
		},
		{
			name:     "dashboard sparkline column with format",
			schemaID: DashboardV1ID,
			yaml: `name: Sparkline Format
rows:
  - widgets:
      - name: Active
        type: table
        sql: SELECT trend FROM accounts
        columns:
          - name: trend
            type: sparkline
            x: { field: day }
            y: { field: amount }
            format:
              - { if: less_than, value: 1, backgroundColor: red }
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err == nil {
				t.Fatalf("expected invalid %s", tt.name)
			}
		})
	}
}

func TestSchemasValidateRepositoryYAML(t *testing.T) {
	validateFiles(t, DashboardV1ID,
		"../examples/*/dashboards/*.yml",
		"../examples/*/dashboards/*.yaml",
		"../testdata/dashboards/*.yml",
		"../testdata/dashboards/*.yaml",
		"../testdata/project/dashboards/*.yml",
		"../testdata/project/dashboards/*.yaml",
	)
	validateFiles(t, ThemeV1ID,
		"../testdata/themes/*.yml",
		"../testdata/themes/*.yaml",
	)
}

func validateFiles(t *testing.T, schemaID string, patterns ...string) {
	t.Helper()

	var matched int
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("bad glob %q: %v", pattern, err)
		}
		for _, file := range files {
			if filepath.Base(file)[0] == '.' {
				continue
			}
			matched++
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if err := ValidateYAML(schemaID, data); err != nil {
				t.Fatalf("%s does not match %s: %v", file, schemaID, err)
			}
		}
	}
	if matched == 0 {
		t.Fatalf("no files matched for schema %s", schemaID)
	}
}
