package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormat_YAML_LegacyScalar(t *testing.T) {
	// Backward compatibility: a scalar `format` is the old value-display
	// shorthand, now equivalent to `number`. It maps into Number with no layers.
	// `number` and an array `format` may coexist (value display + coloring).
	yamlBody := `
columns:
  - name: revenue
    format: currency
  - name: score
    number: number
    format:
      - { if: greater_than_or_equal, value: 80, backgroundColor: green }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[0].Number != "currency" {
		t.Fatalf("scalar format should map to Number, got %q", w.Columns[0].Number)
	}
	if len(w.Columns[0].Format) != 0 {
		t.Fatalf("scalar format should not populate layers, got %d", len(w.Columns[0].Format))
	}
	if w.Columns[1].Number != "number" || len(w.Columns[1].Format) != 1 {
		t.Fatalf("number+layers should coexist: number=%q layers=%d", w.Columns[1].Number, len(w.Columns[1].Format))
	}
}

func TestFormat_YAML_Alias(t *testing.T) {
	// A YAML alias (`format: *anchor`) to an anchored scalar or sequence must
	// resolve to its target, not be rejected.
	yamlBody := `
columns:
  - name: a
    format: &fmt currency
  - name: b
    format: *fmt
  - name: c
    format: &layers
      - { if: greater_than, value: 0, backgroundColor: green }
  - name: d
    format: *layers
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[1].Number != "currency" {
		t.Fatalf("aliased scalar format should map to Number, got %q", w.Columns[1].Number)
	}
	if len(w.Columns[3].Format) != 1 {
		t.Fatalf("aliased sequence format should decode to layers, got %d", len(w.Columns[3].Format))
	}
}

func TestFormat_YAML_Layers(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    number: "$,.2f"
    format:
      - backgroundColor: [red, white, green]
        range: [-25, 0, 25]
        unit: absolute
  - name: region
    format:
      - { backgroundColor: "#F8FAFC", bold: true }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[0].Number != "$,.2f" {
		t.Fatalf("expected column-level number, got %q", w.Columns[0].Number)
	}
	layers := w.Columns[0].Format
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}
	colors, ok := layers[0].BackgroundColor.([]interface{})
	if !ok || len(colors) != 3 {
		t.Errorf("expected gradient array, got %#v", layers[0].BackgroundColor)
	}
	if layers[0].Unit != "absolute" || len(layers[0].Range) != 3 || layers[0].Range[0] != -25 || layers[0].Range[2] != 25 {
		t.Errorf("unexpected range/unit: %+v", layers[0])
	}
	flat := w.Columns[1].Format
	if len(flat) != 1 || flat[0].BackgroundColor != "#F8FAFC" || !flat[0].Bold {
		t.Errorf("unexpected flat layer: %+v", flat)
	}
}

func TestFormat_YAML_Conditions(t *testing.T) {
	yamlBody := `
columns:
  - name: status
    format:
      - { if: text_is_exactly, value: overdue, backgroundColor: red, bold: true }
      - { if: is_between, value: [10, 20], backgroundColor: amber }
      - { if: less_than, value: { column: target }, backgroundColor: red }
      - { backgroundColor: [red, white, green] }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	layers := w.Columns[0].Format
	if len(layers) != 4 {
		t.Fatalf("expected 4 layers, got %d", len(layers))
	}
	if layers[0].If != CondTextIsExactly || layers[0].Value != "overdue" || layers[0].BackgroundColor != "red" || !layers[0].Bold {
		t.Errorf("unexpected layer 0: %+v", layers[0])
	}
	if pair, ok := layers[1].Value.([]interface{}); !ok || len(pair) != 2 {
		t.Errorf("expected two-element between value, got %+v", layers[1].Value)
	}
	ref, ok := layers[2].Value.(map[string]interface{})
	if !ok || ref["column"] != "target" {
		t.Errorf("expected cross-column ref, got %+v", layers[2].Value)
	}
	// the last (if-less) layer is the gradient base
	if layers[3].If != "" {
		t.Errorf("expected base layer without if, got %+v", layers[3])
	}
}

func TestFormat_JSON_RoundTrip(t *testing.T) {
	body := []byte(`{"columns":[{"name":"r","number":"currency","format":[{"backgroundColor":["red","green"]}]}]}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[0].Number != "currency" {
		t.Fatalf("unexpected number: %q", w.Columns[0].Number)
	}
	layers := w.Columns[0].Format
	if len(layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(layers))
	}
	if colors, ok := layers[0].BackgroundColor.([]interface{}); !ok || len(colors) != 2 {
		t.Errorf("expected 2-color gradient, got %#v", layers[0].BackgroundColor)
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
          { name: "revenue", number: "currency", format: [{ backgroundColor: ["red", "white", "green"] }] },
          { name: "status", format: [
              { if: "text_contains", value: "fail", backgroundColor: "red", bold: true },
            ] },
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
	if cols[1].Number != "currency" {
		t.Fatalf("unexpected revenue number: %q", cols[1].Number)
	}
	if colors, ok := cols[1].Format[0].BackgroundColor.([]interface{}); !ok || len(colors) != 3 {
		t.Errorf("expected 3-color gradient, got %#v", cols[1].Format[0].BackgroundColor)
	}
	sc := cols[2].Format
	if len(sc) != 1 || sc[0].If != CondTextContains || !sc[0].Bold {
		t.Errorf("unexpected status layers: %+v", sc)
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

func validateColumnFormat(t *testing.T, layers []FormatLayer) []string {
	t.Helper()
	return runValidate(&Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "c", Format: layers}},
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

func TestValidate_RangeNeedsGradient(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{BackgroundColor: "#fff", Range: []float64{0, 100}}})
	if !hasErrContaining(errs, "range: requires a gradient") {
		t.Errorf("expected range-needs-gradient error, got %v", errs)
	}
}

func TestValidate_RangeLengthMismatch(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{
		BackgroundColor: []interface{}{"red", "green"},
		Range:           []float64{0, 50, 100},
		Unit:            "absolute",
	}})
	if !hasErrContaining(errs, "must match") {
		t.Errorf("expected range-length error, got %v", errs)
	}
}

func TestValidate_ConditionUnknownOperator(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{If: "bogus", BackgroundColor: "red"}})
	if !hasErrContaining(errs, "unknown operator \"bogus\"") {
		t.Errorf("expected unknown-operator error, got %v", errs)
	}
}

func TestValidate_ConditionBetweenNeedsPair(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{If: CondIsBetween, Value: 5, BackgroundColor: "red"}})
	if !hasErrContaining(errs, "requires a two-element value list") {
		t.Errorf("expected between-pair error, got %v", errs)
	}
}

func TestValidate_LayerRequiresStyle(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{If: CondGreaterThan, Value: 5}})
	if !hasErrContaining(errs, "at least one style") {
		t.Errorf("expected style-required error, got %v", errs)
	}
}

func TestValidate_GradientNeedsTwoColors(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{BackgroundColor: []interface{}{"red"}}})
	if !hasErrContaining(errs, "a gradient needs at least 2 colors") {
		t.Errorf("expected min-2-colors error, got %v", errs)
	}
}

func TestValidate_UnknownUnit(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{
		BackgroundColor: []interface{}{"red", "green"},
		Range:           []float64{0, 100},
		Unit:            "bogus",
	}})
	if !hasErrContaining(errs, "unit: unknown unit \"bogus\"") {
		t.Errorf("expected unit error, got %v", errs)
	}
}

func TestValidate_PercentileRange(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{
		BackgroundColor: []interface{}{"red", "white", "green"},
		Range:           []float64{0, 50, 150},
		Unit:            "percentile",
	}})
	if !hasErrContaining(errs, "percentile anchors must be between 0 and 100") {
		t.Errorf("expected percentile-range error, got %v", errs)
	}
}

func TestValidate_PercentileValid(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{{
		BackgroundColor: []interface{}{"red", "white", "green"},
		Range:           []float64{0, 50, 100},
		Unit:            "percentile",
	}})
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestFormat_YAML_Like(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    number: currency
    format:
      - { backgroundColor: [red, white, green] }
  - name: profit
    number: currency
    like: revenue
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[1].Like != "revenue" {
		t.Fatalf("expected like=revenue, got %q", w.Columns[1].Like)
	}
}

func TestValidate_LikeUnknownColumn(t *testing.T) {
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "profit", Like: "revenue"}},
		}}}},
	}
	errs := runValidate(d)
	if !hasErrContaining(errs, "like: references unknown column \"revenue\"") {
		t.Errorf("expected like-unknown error, got %v", errs)
	}
}

func TestFormat_YAML_Hidden(t *testing.T) {
	yamlBody := `
columns:
  - name: actual
    number: number
    format:
      - { if: less_than, value: { column: target }, backgroundColor: red }
  - name: target
    number: number
    hidden: true
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Columns[0].Hidden {
		t.Fatalf("actual should not be hidden")
	}
	if !w.Columns[1].Hidden {
		t.Fatalf("target should be hidden")
	}
}

func TestValidate_LikeHiddenColumn(t *testing.T) {
	// A `like` source may be a hidden column: it stays in the result (and the
	// declared column set) so the reference resolves even though it isn't rendered.
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{
				{Name: "score", Hidden: true, Format: []FormatLayer{{BackgroundColor: []interface{}{"red", "green"}}}},
				{Name: "bonus", Like: "score"},
			},
		}}}},
	}
	errs := runValidate(d)
	if hasErrContaining(errs, "unknown column") {
		t.Errorf("hidden column should be a valid like source, got %v", errs)
	}
}

func TestValidate_LikeSelfReference(t *testing.T) {
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "a", Like: "a"}},
		}}}},
	}
	errs := runValidate(d)
	if !hasErrContaining(errs, "cannot reference itself") {
		t.Errorf("expected self-reference error, got %v", errs)
	}
}

func TestValidate_FormatValidPasses(t *testing.T) {
	errs := validateColumnFormat(t, []FormatLayer{
		{If: CondGreaterThan, Value: 100, BackgroundColor: "green", Bold: true},
		{If: CondIsBetween, Value: []interface{}{10, 20}, TextColor: "amber"},
		{If: CondIsEmpty, Italic: true},
		{BackgroundColor: []interface{}{"red", "white", "green"}, Range: []float64{-25, 0, 25}, Unit: "absolute"},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}
