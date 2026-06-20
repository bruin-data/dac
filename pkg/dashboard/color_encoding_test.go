package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestColorEncoding_YAML_ObjectForm(t *testing.T) {
	yamlBody := `
color:
  field: region
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Color == nil {
		t.Fatal("expected Color to be populated")
	}
	if w.Color.Field != "region" {
		t.Errorf("expected Field = %q, got %q", "region", w.Color.Field)
	}
	if w.ColorField() != "region" {
		t.Errorf("ColorField() = %q, want %q", w.ColorField(), "region")
	}
}

func TestColorEncoding_YAML_BareStringRejected(t *testing.T) {
	yamlBody := `color: region`
	var w Widget
	err := yaml.Unmarshal([]byte(yamlBody), &w)
	if err == nil {
		t.Fatal("expected error for bare-string color, got nil")
	}
}

func TestColorEncoding_JSON_ObjectForm(t *testing.T) {
	body := []byte(`{"color": {"field": "region"}}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.ColorField() != "region" {
		t.Errorf("expected region, got %q", w.ColorField())
	}
}

func TestColorEncoding_JSON_MarshalObject(t *testing.T) {
	c := &ColorEncoding{Field: "region"}
	out, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"field":"region"}`
	if string(out) != want {
		t.Errorf("expected %s, got %s", want, string(out))
	}
}

func TestColorEncoding_ColorField_NilSafe(t *testing.T) {
	var w Widget
	if w.ColorField() != "" {
		t.Errorf("expected empty string for nil Color, got %q", w.ColorField())
	}
}

func TestValidate_ColorOnlyOnBarLineArea(t *testing.T) {
	cases := map[string]bool{
		"bar":    true,
		"line":   true,
		"area":   true,
		"pie":    false,
		"gauge":  false,
		"sankey": false,
	}
	for chart, ok := range cases {
		t.Run(chart, func(t *testing.T) {
			d := &Dashboard{
				Name: "t",
				Rows: []Row{{Widgets: []Widget{{
					Name:  "w",
					Type:  WidgetTypeChart,
					Chart: chart,
					SQL:   "SELECT 1 AS x, 'a' AS r, 1 AS y",
					X:     &AxisEncoding{Field: "x"},
					Y:     &AxisEncoding{Field: "y"},
					Value: &ValueEncoding{Field: "y"},
					Color: &ColorEncoding{Field: "r"},
				}}}},
			}
			err := Validate(d)
			hasColorErr := err != nil && strings.Contains(err.Error(), "color is only valid on bar, line, or area charts")
			if ok && hasColorErr {
				t.Errorf("chart %q: expected color to be accepted, got error: %v", chart, err)
			}
			if !ok && !hasColorErr {
				t.Errorf("chart %q: expected color rejection, got: %v", chart, err)
			}
		})
	}
}

func TestValidate_NormalizedRequiresStacked(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:       "w",
			Type:       WidgetTypeChart,
			Chart:      "bar",
			SQL:        "SELECT 1 AS x, 1 AS y",
			X:          &AxisEncoding{Field: "x"},
			Y:          &AxisEncoding{Field: "y"},
			Normalized: true,
		}}}},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "normalized requires stacked") {
		t.Fatalf("expected normalized-requires-stacked error, got: %v", err)
	}
}

func TestValidate_NormalizedOnlyOnBar(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:       "w",
			Type:       WidgetTypeChart,
			Chart:      "line",
			SQL:        "SELECT 1 AS x, 1 AS y",
			X:          &AxisEncoding{Field: "x"},
			Y:          &AxisEncoding{Field: "y"},
			Stacked:    true,
			Normalized: true,
		}}}},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "normalized is only valid on bar charts") {
		t.Fatalf("expected normalized-bar-only error, got: %v", err)
	}
}

func TestValidate_HorizontalOnlyOnBar(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:       "w",
			Type:       WidgetTypeChart,
			Chart:      "line",
			SQL:        "SELECT 1 AS x, 1 AS y",
			X:          &AxisEncoding{Field: "x"},
			Y:          &AxisEncoding{Field: "y"},
			Horizontal: true,
		}}}},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "horizontal is only valid on bar charts") {
		t.Fatalf("expected horizontal-bar-only error, got: %v", err)
	}
}

func TestValidate_HappyPath_BarWithColorStackedNormalizedHorizontal(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:       "w",
			Type:       WidgetTypeChart,
			Chart:      "bar",
			SQL:        "SELECT month, region, SUM(amount) AS revenue FROM sales GROUP BY 1, 2",
			X:          &AxisEncoding{Field: "month"},
			Y:          &AxisEncoding{Field: "revenue"},
			Color:      &ColorEncoding{Field: "region"},
			Stacked:    true,
			Normalized: true,
			Horizontal: true,
		}}}},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_ColorFieldRequired(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:  "w",
			Type:  WidgetTypeChart,
			Chart: "bar",
			SQL:   "SELECT 1 AS x, 1 AS y",
			X:     &AxisEncoding{Field: "x"},
			Y:     &AxisEncoding{Field: "y"},
			Color: &ColorEncoding{Field: ""},
		}}}},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "color.field is required") {
		t.Fatalf("expected color.field-required error, got: %v", err)
	}
}

func TestValidate_ColorWithMultipleYRejected(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{
			Name:  "w",
			Type:  WidgetTypeChart,
			Chart: "bar",
			SQL:   "SELECT 1 AS x, 1 AS a, 1 AS b",
			X:     &AxisEncoding{Field: "x"},
			Y:     &AxisEncoding{Field: []string{"a", "b"}},
			Color: &ColorEncoding{Field: "r"},
		}}}},
	}
	err := Validate(d)
	if err == nil || !strings.Contains(err.Error(), "color with multiple y fields is not supported") {
		t.Fatalf("expected color+multi-y error, got: %v", err)
	}
}
