package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormat_YAML_StringShorthand(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    format: currency
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f := w.Columns[0].Format
	if f == nil || f.Number != "currency" {
		t.Fatalf("expected string shorthand → Number=currency, got %+v", f)
	}
}

func TestFormat_YAML_Object(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    format:
      number: "$,.2f"
      domain: [-25, 0, 25]
      backgroundColor: [red, white, green]
  - name: region
    format: { backgroundColor: "#F8FAFC", bold: true }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f := w.Columns[0].Format
	if f == nil || f.Number != "$,.2f" {
		t.Fatalf("unexpected format: %+v", f)
	}
	if f.Domain == nil || len(f.Domain.Anchors) != 3 || f.Domain.Anchors[0] != -25 || f.Domain.Anchors[2] != 25 {
		t.Errorf("unexpected domain: %+v", f.Domain)
	}
	colors, ok := f.BackgroundColor.([]interface{})
	if !ok || len(colors) != 3 {
		t.Errorf("expected gradient array, got %#v", f.BackgroundColor)
	}
	flat := w.Columns[1].Format
	if flat == nil || flat.BackgroundColor != "#F8FAFC" || !flat.Bold {
		t.Errorf("unexpected flat format: %+v", flat)
	}
}

func TestFormat_YAML_Rules(t *testing.T) {
	yamlBody := `
columns:
  - name: status
    format:
      rules:
        - { if: text_is_exactly, value: overdue, backgroundColor: red, bold: true }
        - { if: is_between, value: [10, 20], backgroundColor: amber }
        - { if: less_than, value: { column: target }, backgroundColor: red }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rules := w.Columns[0].Format.Rules
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}
	if rules[0].If != CondTextIsExactly || rules[0].Value != "overdue" || rules[0].BackgroundColor != "red" || !rules[0].Bold {
		t.Errorf("unexpected rule 0: %+v", rules[0])
	}
	if pair, ok := rules[1].Value.([]interface{}); !ok || len(pair) != 2 {
		t.Errorf("expected two-element between value, got %+v", rules[1].Value)
	}
	ref, ok := rules[2].Value.(map[string]interface{})
	if !ok || ref["column"] != "target" {
		t.Errorf("expected cross-column ref, got %+v", rules[2].Value)
	}
}

func TestFormat_JSON_RoundTrip(t *testing.T) {
	body := []byte(`{"columns":[{"name":"r","format":{"number":"currency","backgroundColor":["red","green"]}}]}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	f := w.Columns[0].Format
	if f == nil || f.Number != "currency" {
		t.Fatalf("unexpected format: %+v", f)
	}
	if colors, ok := f.BackgroundColor.([]interface{}); !ok || len(colors) != 2 {
		t.Errorf("expected 2-color gradient, got %#v", f.BackgroundColor)
	}
}

func TestFormat_JSON_StringShorthand(t *testing.T) {
	var w Widget
	if err := json.Unmarshal([]byte(`{"columns":[{"name":"r","format":"number"}]}`), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[0].Format.Number != "number" {
		t.Errorf("expected Number=number, got %+v", w.Columns[0].Format)
	}
}

func TestFormat_TSX(t *testing.T) {
	source := `
export default (
  <Dashboard name="Heat" connection="db">
    <Row>
      <Table name="Sales" col={12}
        sql="SELECT region, revenue, status FROM sales"
        columns={[
          { name: "region", label: "Region" },
          { name: "revenue", format: { number: "currency", backgroundColor: ["red", "white", "green"] } },
          { name: "status", format: {
              rules: [
                { if: "text_contains", value: "fail", backgroundColor: "red", bold: true },
              ],
            } },
        ]} />
      <Chart name="Sales" chart="bar" col={12}
        sql="SELECT m, a, b FROM t" x="m" y={["a", "b"]} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)

	cols := d.Rows[0].Widgets[0].Columns
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	f := cols[1].Format
	if f == nil || f.Number != "currency" {
		t.Fatalf("unexpected revenue format: %+v", f)
	}
	if colors, ok := f.BackgroundColor.([]interface{}); !ok || len(colors) != 3 {
		t.Errorf("expected 3-color gradient, got %#v", f.BackgroundColor)
	}
	sc := cols[2].Format
	if sc == nil || len(sc.Rules) != 1 || sc.Rules[0].If != CondTextContains || !sc.Rules[0].Bold {
		t.Errorf("unexpected status rules: %+v", sc)
	}
}

// --- validation ---

func runValidate(d *Dashboard) []string {
	err := Validate(d)
	if err == nil {
		return nil
	}
	return err.(*ValidationError).Errors
}

func validateColumnFormat(t *testing.T, f *Format) []string {
	t.Helper()
	return runValidate(&Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "c", Format: f}},
		}}}},
	})
}

func hasErrContaining(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestValidate_UnknownScheme(t *testing.T) {
	errs := validateColumnFormat(t, &Format{Scheme: "rainbow"})
	if !hasErrContaining(errs, "unknown scheme") {
		t.Errorf("expected unknown-scheme error, got %v", errs)
	}
}

func TestValidate_DomainNeedsGradient(t *testing.T) {
	errs := validateColumnFormat(t, &Format{Domain: &Domain{Anchors: []float64{0, 100}}, BackgroundColor: "#fff"})
	if !hasErrContaining(errs, "domain: requires a gradient") {
		t.Errorf("expected domain-needs-gradient error, got %v", errs)
	}
}

func TestValidate_DomainWithSchemeAccepted(t *testing.T) {
	// A built-in scheme is a gradient, so a matching domain is valid.
	errs := validateColumnFormat(t, &Format{Scheme: "red-white-green", Domain: &Domain{Anchors: []float64{-25, 0, 25}}})
	if len(errs) != 0 {
		t.Errorf("expected domain+scheme to pass, got %v", errs)
	}
}

func TestValidate_DomainSchemeLengthMismatch(t *testing.T) {
	// white-green has 2 colors, so a 3-value domain must not match.
	errs := validateColumnFormat(t, &Format{Scheme: "white-green", Domain: &Domain{Anchors: []float64{0, 50, 100}}})
	if !hasErrContaining(errs, "must match") {
		t.Errorf("expected length-mismatch error, got %v", errs)
	}
}

func TestValidate_DomainWithPaletteAccepted(t *testing.T) {
	// A custom palette is a gradient too; a matching domain is valid.
	d := paletteDashboard(map[string][]string{"risk": {"red", "amber", "green"}}, "risk")
	d.Rows[0].Widgets[0].Columns[0].Format.Domain = &Domain{Anchors: []float64{-25, 0, 25}}
	if errs := runValidate(d); len(errs) != 0 {
		t.Errorf("expected domain+palette to pass, got %v", errs)
	}
}

func TestValidate_DomainLengthMismatch(t *testing.T) {
	errs := validateColumnFormat(t, &Format{
		Domain:          &Domain{Anchors: []float64{0, 50, 100}},
		BackgroundColor: []interface{}{"red", "green"},
	})
	if !hasErrContaining(errs, "must match") {
		t.Errorf("expected domain-length error, got %v", errs)
	}
}

func TestValidate_RuleUnknownOperator(t *testing.T) {
	errs := validateColumnFormat(t, &Format{Rules: []FormatRule{{If: "bogus", BackgroundColor: "red"}}})
	if !hasErrContaining(errs, "unknown operator \"bogus\"") {
		t.Errorf("expected unknown-operator error, got %v", errs)
	}
}

func TestValidate_RuleBetweenNeedsPair(t *testing.T) {
	errs := validateColumnFormat(t, &Format{Rules: []FormatRule{{If: CondIsBetween, Value: 5, BackgroundColor: "red"}}})
	if !hasErrContaining(errs, "requires a two-element value list") {
		t.Errorf("expected between-pair error, got %v", errs)
	}
}

func TestValidate_RuleRequiresStyle(t *testing.T) {
	errs := validateColumnFormat(t, &Format{Rules: []FormatRule{{If: CondGreaterThan, Value: 5}}})
	if !hasErrContaining(errs, "at least one style") {
		t.Errorf("expected style-required error, got %v", errs)
	}
}

func TestValidate_GradientNeedsTwoColors(t *testing.T) {
	errs := validateColumnFormat(t, &Format{BackgroundColor: []interface{}{"red"}})
	if !hasErrContaining(errs, "a gradient needs at least 2 colors") {
		t.Errorf("expected min-2-colors error, got %v", errs)
	}
}

func TestValidate_DomainUnknownUnit(t *testing.T) {
	errs := validateColumnFormat(t, &Format{
		Domain:          &Domain{Unit: "bogus"},
		BackgroundColor: []interface{}{"red", "green"},
	})
	if !hasErrContaining(errs, "domain.unit: unknown unit \"bogus\"") {
		t.Errorf("expected domain.unit error, got %v", errs)
	}
}

func TestValidate_PercentileDomainRange(t *testing.T) {
	errs := validateColumnFormat(t, &Format{
		Domain:          &Domain{Unit: "percentile", Anchors: []float64{0, 50, 150}},
		BackgroundColor: []interface{}{"red", "white", "green"},
	})
	if !hasErrContaining(errs, "percentile anchors must be between 0 and 100") {
		t.Errorf("expected percentile-range error, got %v", errs)
	}
}

func TestValidate_DomainPercentileValid(t *testing.T) {
	errs := validateColumnFormat(t, &Format{
		Domain:          &Domain{Unit: "percentile", Anchors: []float64{0, 50, 100}},
		BackgroundColor: []interface{}{"red", "white", "green"},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestFormat_YAML_DomainObject(t *testing.T) {
	yamlBody := `
columns:
  - name: a
    format:
      domain: [-25, 0, 25]
      backgroundColor: [red, white, green]
  - name: b
    format:
      domain: { unit: percentile, anchors: [0, 50, 100] }
      backgroundColor: [red, white, green]
  - name: c
    format:
      domain: { unit: percentile }
      backgroundColor: [red, white, green]
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d := w.Columns[0].Format.Domain; d == nil || d.Unit != "" || len(d.Anchors) != 3 {
		t.Errorf("array form: expected raw values, got %+v", d)
	}
	if d := w.Columns[1].Format.Domain; d == nil || d.Unit != "percentile" || len(d.Anchors) != 3 {
		t.Errorf("object form: expected percentile + 3 values, got %+v", d)
	}
	if d := w.Columns[2].Format.Domain; d == nil || d.Unit != "percentile" || len(d.Anchors) != 0 {
		t.Errorf("unit-only form: expected percentile, no values, got %+v", d)
	}
}

func TestFormat_YAML_Like(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    format: { number: currency, backgroundColor: [red, white, green] }
  - name: profit
    format: { like: revenue }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[1].Format == nil || w.Columns[1].Format.Like != "revenue" {
		t.Fatalf("expected like=revenue, got %+v", w.Columns[1].Format)
	}
}

func TestValidate_LikeUnknownColumn(t *testing.T) {
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "profit", Format: &Format{Like: "revenue"}}},
		}}}},
	}
	errs := runValidate(d)
	if !hasErrContaining(errs, "like: references unknown column \"revenue\"") {
		t.Errorf("expected like-unknown error, got %v", errs)
	}
}

func TestValidate_LikeSelfReference(t *testing.T) {
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "a", Format: &Format{Like: "a"}}},
		}}}},
	}
	errs := runValidate(d)
	if !hasErrContaining(errs, "cannot reference itself") {
		t.Errorf("expected self-reference error, got %v", errs)
	}
}

func TestValidate_FormatValidPasses(t *testing.T) {
	errs := validateColumnFormat(t, &Format{
		Number:          "currency",
		Domain:          &Domain{Anchors: []float64{-25, 0, 25}},
		BackgroundColor: []interface{}{"red", "white", "green"},
		Rules: []FormatRule{
			{If: CondGreaterThan, Value: 100, BackgroundColor: "green", Bold: true},
			{If: CondIsBetween, Value: []interface{}{10, 20}, TextColor: "amber"},
			{If: CondIsEmpty, Italic: true},
		},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

// --- custom palettes ---

// paletteDashboard builds a dashboard with the given palettes and one table
// column whose format references a scheme by name.
func paletteDashboard(palettes map[string][]string, scheme string) *Dashboard {
	return &Dashboard{
		Name:     "d",
		Palettes: palettes,
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "c", Format: &Format{Scheme: scheme}}},
		}}}},
	}
}

func TestFormat_YAML_Palettes(t *testing.T) {
	yamlBody := `
name: Palettes
palettes:
  risk: [red, amber, green]
  heat: ["#FEF0D9", "#E34A33"]
rows:
  - widgets:
      - name: t
        type: table
        sql: SELECT 1
        columns:
          - name: c
            format: { scheme: risk }
`
	var d Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := d.Palettes["risk"]; len(got) != 3 || got[0] != "red" || got[2] != "green" {
		t.Errorf("unexpected risk palette: %v", d.Palettes["risk"])
	}
	if len(d.Palettes["heat"]) != 2 {
		t.Errorf("unexpected heat palette: %v", d.Palettes["heat"])
	}
}

func TestFormat_TSX_Palettes(t *testing.T) {
	source := `
export default (
  <Dashboard name="Heat" palettes={{ risk: ["red", "amber", "green"] }}>
    <Row>
      <Table name="Sales" col={12} sql="SELECT region, revenue FROM sales"
        columns={[
          { name: "revenue", format: { scheme: "risk" } },
        ]} />
    </Row>
  </Dashboard>
)
`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)
	if got := d.Palettes["risk"]; len(got) != 3 || got[1] != "amber" {
		t.Errorf("unexpected risk palette from TSX: %v", d.Palettes["risk"])
	}
}

func TestValidate_PaletteSchemeAccepted(t *testing.T) {
	errs := runValidate(paletteDashboard(map[string][]string{"risk": {"red", "amber", "green"}}, "risk"))
	if len(errs) != 0 {
		t.Errorf("expected palette scheme to pass, got %v", errs)
	}
}

func TestValidate_PaletteUnknownScheme(t *testing.T) {
	errs := runValidate(paletteDashboard(map[string][]string{"risk": {"red", "amber", "green"}}, "nope"))
	if !hasErrContaining(errs, "unknown scheme") {
		t.Errorf("expected unknown-scheme error, got %v", errs)
	}
}

func TestValidate_PaletteTooFewColors(t *testing.T) {
	errs := runValidate(paletteDashboard(map[string][]string{"solo": {"red"}}, "solo"))
	if !hasErrContaining(errs, "at least 2 colors") {
		t.Errorf("expected too-few-colors error, got %v", errs)
	}
}

func TestValidate_PaletteCollidesWithBuiltin(t *testing.T) {
	errs := runValidate(paletteDashboard(map[string][]string{"white-green": {"red", "green"}}, "white-green"))
	if !hasErrContaining(errs, "collides with a built-in") {
		t.Errorf("expected collision error, got %v", errs)
	}
}
