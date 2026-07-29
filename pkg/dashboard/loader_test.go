package dashboard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
