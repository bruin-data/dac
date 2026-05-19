package dashboard

import (
	"encoding/json"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAxisEncoding_YAML_ScalarField(t *testing.T) {
	yamlBody := `x: month`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.X == nil {
		t.Fatal("expected X to be populated")
	}
	if got, ok := w.X.Field.(string); !ok || got != "month" {
		t.Errorf("expected Field = \"month\" (string), got %T %v", w.X.Field, w.X.Field)
	}
	if w.X.Type != "" || w.X.Title != "" || w.X.Format != "" {
		t.Errorf("expected no metadata, got %+v", w.X)
	}
	if w.XField() != "month" {
		t.Errorf("XField() = %q, want %q", w.XField(), "month")
	}
}

func TestAxisEncoding_YAML_ListField(t *testing.T) {
	yamlBody := `y: [revenue, cost]`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Y == nil {
		t.Fatal("expected Y to be populated")
	}
	got, ok := w.Y.Field.([]string)
	if !ok {
		t.Fatalf("expected Field to be []string, got %T", w.Y.Field)
	}
	want := []string{"revenue", "cost"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected Field = %v, got %v", want, got)
	}
	if ys := w.YFields(); !reflect.DeepEqual(ys, want) {
		t.Errorf("YFields() = %v, want %v", ys, want)
	}
}

func TestAxisEncoding_YAML_ObjectForm(t *testing.T) {
	yamlBody := `
x:
  field: month
  type: date
  format: "%b %Y"
  title: Month
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.X == nil {
		t.Fatal("expected X to be populated")
	}
	if got, ok := w.X.Field.(string); !ok || got != "month" {
		t.Errorf("expected Field = \"month\", got %T %v", w.X.Field, w.X.Field)
	}
	if w.X.Type != "date" {
		t.Errorf("expected Type = date, got %q", w.X.Type)
	}
	if w.X.Format != "%b %Y" {
		t.Errorf("expected Format = %q, got %q", "%b %Y", w.X.Format)
	}
	if w.X.Title != "Month" {
		t.Errorf("expected Title = Month, got %q", w.X.Title)
	}
}

func TestAxisEncoding_YAML_ObjectFormWithFieldList(t *testing.T) {
	yamlBody := `
y:
  field: [revenue, cost]
  type: number
  format: ",.2f"
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Y == nil {
		t.Fatal("expected Y to be populated")
	}
	got, ok := w.Y.Field.([]string)
	if !ok {
		t.Fatalf("expected Field to be []string, got %T", w.Y.Field)
	}
	if !reflect.DeepEqual(got, []string{"revenue", "cost"}) {
		t.Errorf("expected [revenue cost], got %v", got)
	}
	if w.Y.Type != "number" || w.Y.Format != ",.2f" {
		t.Errorf("expected metadata Type=number Format=,.2f, got %+v", w.Y)
	}
}

func TestAxisEncoding_JSON_ScalarField(t *testing.T) {
	body := []byte(`{"x": "month"}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.XField() != "month" {
		t.Errorf("expected month, got %q", w.XField())
	}
}

func TestAxisEncoding_JSON_ListField(t *testing.T) {
	body := []byte(`{"y": ["revenue", "cost"]}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"revenue", "cost"}
	if !reflect.DeepEqual(w.YFields(), want) {
		t.Errorf("expected %v, got %v", want, w.YFields())
	}
}

func TestAxisEncoding_JSON_ObjectForm(t *testing.T) {
	body := []byte(`{"x": {"field": "month", "type": "date", "format": "%b %Y", "title": "Month"}}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.X == nil {
		t.Fatal("expected X to be populated")
	}
	if w.XField() != "month" {
		t.Errorf("expected month, got %q", w.XField())
	}
	if w.X.Type != "date" || w.X.Format != "%b %Y" || w.X.Title != "Month" {
		t.Errorf("expected metadata, got %+v", w.X)
	}
}

func TestAxisEncoding_JSON_ObjectFormWithFieldList(t *testing.T) {
	body := []byte(`{"y": {"field": ["revenue", "cost"], "type": "number"}}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"revenue", "cost"}
	if !reflect.DeepEqual(w.YFields(), want) {
		t.Errorf("expected %v, got %v", want, w.YFields())
	}
	if w.Y.Type != "number" {
		t.Errorf("expected Type=number, got %q", w.Y.Type)
	}
}

func TestAxisEncoding_JSON_MarshalBareWhenNoMetadata(t *testing.T) {
	w := Widget{X: &AxisEncoding{Field: "month"}, Y: &AxisEncoding{Field: []string{"revenue", "cost"}}}
	out, err := json.Marshal(struct {
		X *AxisEncoding `json:"x"`
		Y *AxisEncoding `json:"y"`
	}{w.X, w.Y})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"x":"month","y":["revenue","cost"]}`
	if string(out) != want {
		t.Errorf("expected %s, got %s", want, string(out))
	}
}

func TestAxisEncoding_JSON_MarshalObjectWhenMetadataPresent(t *testing.T) {
	a := &AxisEncoding{Field: "month", Type: "date", Format: "%b %Y"}
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if got["field"] != "month" || got["type"] != "date" || got["format"] != "%b %Y" {
		t.Errorf("expected object form with field/type/format, got %v", got)
	}
}

func TestAxisEncoding_YAML_RoundTripBare(t *testing.T) {
	yamlIn := "x: month\n"
	var w struct {
		X *AxisEncoding `yaml:"x"`
	}
	if err := yaml.Unmarshal([]byte(yamlIn), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out, err := yaml.Marshal(&w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != yamlIn {
		t.Errorf("expected round-trip %q, got %q", yamlIn, string(out))
	}
}
