package dashboard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Validator tests
// ---------------------------------------------------------------------------

func TestValidate_ValidDashboard(t *testing.T) {
	d, err := LoadFile("../../testdata/dashboards/sales.yml")
	assertNoErr(t, err)

	err = Validate(d)
	assertNoErr(t, err)
}

func TestValidate_MissingName(t *testing.T) {
	d := &Dashboard{
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "name is required")
}

func TestValidate_MissingRows(t *testing.T) {
	d := &Dashboard{Name: "test"}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "at least one row is required")
}

func TestValidate_EmptyRow(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "at least one widget is required")
}

func TestValidate_WidgetMissingName(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "name is required")
}

func TestValidate_WidgetMissingType(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "type is required")
}

func TestValidate_ColumnSumExceeds12(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{
				{Name: "a", Type: WidgetTypeText, Content: "hi", Col: 8},
				{Name: "b", Type: WidgetTypeText, Content: "hi", Col: 6},
			}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "exceeds 12")
}

func TestValidate_TextWidgetMissingContent(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "content is required")
}

func TestValidate_ChartWidgetMissingChartType(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeChart, SQL: "SELECT 1"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "chart type is required")
}

func TestValidate_FilterTypes(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Filters: []Filter{
			{Name: "region", Type: "select"},
			{Name: "date_range", Type: "date-range"},
			{Name: "as_of_date", Type: "date"},
			{Name: "min_revenue", Type: "number"},
			{Name: "search", Type: "text"},
		},
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	assertNoErr(t, Validate(d))

	d.Filters = append(d.Filters, Filter{Name: "bad", Type: "boolean"})
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, `unknown filter type "boolean"`)
}

func TestValidate_ProjectSemanticDashboard(t *testing.T) {
	d, err := LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	assertNoErr(t, err)

	err = Validate(d)
	assertNoErr(t, err)
}

func TestValidate_ProjectSemanticDashboardMissingModel(t *testing.T) {
	d, err := LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	assertNoErr(t, err)

	d.Model = "missing_model"
	err = Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "semantic model \"missing_model\" not found")
}

func TestValidate_InvalidExternalSemanticModelOnlyFailsReferencedDashboard(t *testing.T) {
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
	semanticDashboard := `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Semantic Dashboard
model: broken_sales
rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
`
	invalidModel := `schema: https://getbruin.com/schemas/dac/semantic-model/v1
name: broken_sales
source:
  table: sales
dimensions:
  - name: revenue
    type: string
metrics:
  - name: revenue
    expression: sum(amount)
`

	assertNoErr(t, os.WriteFile(filepath.Join(dashboardsDir, "regular.yml"), []byte(regularDashboard), 0o644))
	assertNoErr(t, os.WriteFile(filepath.Join(dashboardsDir, "semantic.yml"), []byte(semanticDashboard), 0o644))
	assertNoErr(t, os.WriteFile(filepath.Join(semanticDir, "broken-sales.yml"), []byte(invalidModel), 0o644))

	dashboards, err := LoadDir(projectDir)
	assertNoErr(t, err)

	regular := FindByName(dashboards, "Regular Dashboard")
	if regular == nil {
		t.Fatal("expected regular dashboard to load")
	}
	assertNoErr(t, Validate(regular))

	semanticDash := FindByName(dashboards, "Semantic Dashboard")
	if semanticDash == nil {
		t.Fatal("expected semantic dashboard to load")
	}

	err = Validate(semanticDash)
	assertErr(t, err)
	assertValidationContains(t, err, `semantic model "broken_sales" is invalid`)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertValidationContains(t *testing.T, err error, substr string) {
	t.Helper()
	var ve *ValidationError
	if errors.As(err, &ve) {
		for _, e := range ve.Errors {
			if strings.Contains(e, substr) {
				return
			}
		}
		t.Errorf("expected validation error containing %q, got errors: %v", substr, ve.Errors)
		return
	}
	if strings.Contains(err.Error(), substr) {
		return
	}
	t.Errorf("expected error containing %q, got: %v", substr, err)
}
