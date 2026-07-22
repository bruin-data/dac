package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestColorScale_YAML_NestedStops(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    format: currency
    colorScale:
      min: { type: min, color: "#F8696B" }
      mid: { type: percentile, value: 50, color: "#FFEB84" }
      max: { type: number, value: 1000, color: "#63BE7B" }
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cs := w.Columns[0].ColorScale
	if cs == nil || cs.Min == nil || cs.Mid == nil || cs.Max == nil {
		t.Fatalf("expected all three stops, got %+v", cs)
	}
	if cs.Min.Type != StopTypeMin || cs.Min.Color != "#F8696B" || cs.Min.Value != nil {
		t.Errorf("unexpected min stop: %+v", cs.Min)
	}
	if cs.Mid.Type != StopTypePercentile || cs.Mid.Value == nil || *cs.Mid.Value != 50 {
		t.Errorf("unexpected mid stop: %+v", cs.Mid)
	}
	if cs.Max.Type != StopTypeNumber || cs.Max.Value == nil || *cs.Max.Value != 1000 {
		t.Errorf("unexpected max stop: %+v", cs.Max)
	}
}

func TestColorScale_YAML_BareStringShorthand(t *testing.T) {
	yamlBody := `
columns:
  - name: revenue
    colorScale:
      min: "#000000"
      max: "#FFFFFF"
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cs := w.Columns[0].ColorScale
	if cs.Min == nil || cs.Min.Color != "#000000" || cs.Min.Type != "" {
		t.Errorf("expected bare-string min color, got %+v", cs.Min)
	}
	if cs.Mid != nil {
		t.Errorf("expected no mid stop, got %+v", cs.Mid)
	}
	if cs.Max == nil || cs.Max.Color != "#FFFFFF" {
		t.Errorf("expected bare-string max color, got %+v", cs.Max)
	}
}

func TestColorScale_JSON_BareStringAndObject(t *testing.T) {
	body := []byte(`{"columns":[{"name":"r","colorScale":{"min":"#000000","max":{"type":"percentile","value":90,"color":"#FFFFFF"}}}]}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cs := w.Columns[0].ColorScale
	if cs.Min == nil || cs.Min.Color != "#000000" {
		t.Errorf("expected bare-string min, got %+v", cs.Min)
	}
	if cs.Max == nil || cs.Max.Type != StopTypePercentile || cs.Max.Value == nil || *cs.Max.Value != 90 {
		t.Errorf("expected percentile max, got %+v", cs.Max)
	}
}

func TestSingleColor_YAML(t *testing.T) {
	yamlBody := `
columns:
  - name: status
    singleColor:
      - if: text_is_exactly
        value: overdue
        background: "#FEE2E2"
        textColor: "#991B1B"
        bold: true
      - if: is_between
        value: [10, 20]
        background: "#D1FAE5"
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rules := w.Columns[0].SingleColor
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].If != CondTextIsExactly || rules[0].Value != "overdue" || !rules[0].Bold {
		t.Errorf("unexpected rule 0: %+v", rules[0])
	}
	if rules[0].Background != "#FEE2E2" || rules[0].TextColor != "#991B1B" {
		t.Errorf("unexpected rule 0 styles: %+v", rules[0])
	}
	pair, ok := rules[1].Value.([]interface{})
	if !ok || len(pair) != 2 {
		t.Errorf("expected two-element value for between, got %+v", rules[1].Value)
	}
}

func TestColorScale_TSX(t *testing.T) {
	source := `
export default (
  <Dashboard name="Heat" connection="db">
    <Row>
      <Table name="Sales" col={12}
        sql="SELECT region, revenue FROM sales"
        columns={[
          { name: "region", label: "Region" },
          { name: "revenue", format: "currency",
            colorScale: {
              min: { type: "min", color: "#F8696B" },
              max: { type: "percent", value: 90, color: "#63BE7B" },
            } },
          { name: "status",
            singleColor: [
              { if: "text_contains", value: "fail", background: "#FEE2E2", bold: true },
            ] },
        ]} />
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
	cs := cols[1].ColorScale
	if cs == nil || cs.Min == nil || cs.Min.Type != StopTypeMin || cs.Min.Color != "#F8696B" {
		t.Fatalf("unexpected colorScale min: %+v", cs)
	}
	if cs.Max == nil || cs.Max.Type != StopTypePercent || cs.Max.Value == nil || *cs.Max.Value != 90 {
		t.Errorf("unexpected colorScale max: %+v", cs.Max)
	}
	if cs.Mid != nil {
		t.Errorf("expected no mid stop, got %+v", cs.Mid)
	}
	sc := cols[2].SingleColor
	if len(sc) != 1 || sc[0].If != CondTextContains || sc[0].Value != "fail" || !sc[0].Bold {
		t.Errorf("unexpected singleColor: %+v", sc)
	}
}

// --- validation ---

func validateColumnColorScale(t *testing.T, cs *ColorScale, singleColor []SingleColorRule) []string {
	t.Helper()
	d := &Dashboard{
		Name: "d",
		Rows: []Row{{Widgets: []Widget{{
			Name: "t", Type: WidgetTypeTable, SQL: "SELECT 1",
			Columns: []TableColumn{{Name: "c", ColorScale: cs, SingleColor: singleColor}},
		}}}},
	}
	err := Validate(d)
	if err == nil {
		return nil
	}
	return err.(*ValidationError).Errors
}

func fptr(f float64) *float64 { return &f }

func hasErrContaining(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestValidate_MinStopCannotBeMaxType(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Min: &ColorStop{Type: StopTypeMax}}, nil)
	if !hasErrContaining(errs, "colorScale.min: type \"max\" not allowed") {
		t.Errorf("expected min-type error, got %v", errs)
	}
}

func TestValidate_MaxStopCannotBeMinType(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Max: &ColorStop{Type: StopTypeMin}}, nil)
	if !hasErrContaining(errs, "colorScale.max: type \"min\" not allowed") {
		t.Errorf("expected max-type error, got %v", errs)
	}
}

func TestValidate_MidStopTypeRestricted(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Mid: &ColorStop{Type: StopTypeMin}}, nil)
	if !hasErrContaining(errs, "colorScale.mid: type \"min\" not allowed") {
		t.Errorf("expected mid-type error, got %v", errs)
	}
}

func TestValidate_ValueNotAllowedForMinMaxType(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Min: &ColorStop{Type: StopTypeMin, Value: fptr(5)}}, nil)
	if !hasErrContaining(errs, "value is not allowed for type \"min\"") {
		t.Errorf("expected value-not-allowed error, got %v", errs)
	}
}

func TestValidate_ValueRequiredForNumberType(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Max: &ColorStop{Type: StopTypeNumber}}, nil)
	if !hasErrContaining(errs, "value is required for type \"number\"") {
		t.Errorf("expected value-required error, got %v", errs)
	}
}

func TestValidate_PercentileRange(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{Mid: &ColorStop{Type: StopTypePercentile, Value: fptr(150)}}, nil)
	if !hasErrContaining(errs, "percentile value must be between 0 and 100") {
		t.Errorf("expected percentile-range error, got %v", errs)
	}
}

func TestValidate_ColorScaleValidPasses(t *testing.T) {
	errs := validateColumnColorScale(t, &ColorScale{
		Min: &ColorStop{Type: StopTypeMin, Color: "#F8696B"},
		Mid: &ColorStop{Type: StopTypePercentile, Value: fptr(50), Color: "#FFEB84"},
		Max: &ColorStop{Type: StopTypeNumber, Value: fptr(100), Color: "#63BE7B"},
	}, nil)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidate_SingleColorUnknownOperator(t *testing.T) {
	errs := validateColumnColorScale(t, nil, []SingleColorRule{{If: "bogus", Background: "#fff"}})
	if !hasErrContaining(errs, "unknown operator \"bogus\"") {
		t.Errorf("expected unknown-operator error, got %v", errs)
	}
}

func TestValidate_SingleColorBetweenNeedsPair(t *testing.T) {
	errs := validateColumnColorScale(t, nil, []SingleColorRule{{If: CondIsBetween, Value: 5, Background: "#fff"}})
	if !hasErrContaining(errs, "requires a two-element value list") {
		t.Errorf("expected between-pair error, got %v", errs)
	}
}

func TestValidate_SingleColorRequiresStyle(t *testing.T) {
	errs := validateColumnColorScale(t, nil, []SingleColorRule{{If: CondGreaterThan, Value: 5}})
	if !hasErrContaining(errs, "at least one style") {
		t.Errorf("expected style-required error, got %v", errs)
	}
}

func TestValidate_SingleColorValidPasses(t *testing.T) {
	errs := validateColumnColorScale(t, nil, []SingleColorRule{
		{If: CondGreaterThan, Value: 100, Background: "#D1FAE5", Bold: true},
		{If: CondIsBetween, Value: []interface{}{10, 20}, TextColor: "#065F46"},
		{If: CondIsEmpty, Italic: true},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}
