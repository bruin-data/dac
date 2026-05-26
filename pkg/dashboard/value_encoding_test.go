package dashboard

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValueEncoding_YAML_ScalarField(t *testing.T) {
	yamlBody := `value: amount`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Value == nil {
		t.Fatal("expected Value to be populated")
	}
	if w.Value.Field != "amount" {
		t.Errorf("expected Field = %q, got %q", "amount", w.Value.Field)
	}
	if w.Value.Type != "" || w.Value.Format != "" {
		t.Errorf("expected no metadata, got %+v", w.Value)
	}
	if w.ValueField() != "amount" {
		t.Errorf("ValueField() = %q, want %q", w.ValueField(), "amount")
	}
}

func TestValueEncoding_YAML_ObjectForm(t *testing.T) {
	yamlBody := `
value:
  field: total_revenue
  type: number
  format: "$,.2f"
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Value == nil {
		t.Fatal("expected Value to be populated")
	}
	if w.Value.Field != "total_revenue" {
		t.Errorf("expected Field = %q, got %q", "total_revenue", w.Value.Field)
	}
	if w.Value.Type != "number" {
		t.Errorf("expected Type = number, got %q", w.Value.Type)
	}
	if w.Value.Format != "$,.2f" {
		t.Errorf("expected Format = %q, got %q", "$,.2f", w.Value.Format)
	}
}

func TestValueEncoding_JSON_ScalarField(t *testing.T) {
	body := []byte(`{"value": "amount"}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.ValueField() != "amount" {
		t.Errorf("expected amount, got %q", w.ValueField())
	}
}

func TestValueEncoding_JSON_ObjectForm(t *testing.T) {
	body := []byte(`{"value": {"field": "total_revenue", "type": "number", "format": "$,.2f"}}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Value == nil {
		t.Fatal("expected Value to be populated")
	}
	if w.ValueField() != "total_revenue" {
		t.Errorf("expected total_revenue, got %q", w.ValueField())
	}
	if w.Value.Type != "number" || w.Value.Format != "$,.2f" {
		t.Errorf("expected metadata, got %+v", w.Value)
	}
}

func TestValueEncoding_JSON_MarshalBareWhenNoMetadata(t *testing.T) {
	w := Widget{Value: &ValueEncoding{Field: "amount"}}
	out, err := json.Marshal(struct {
		Value *ValueEncoding `json:"value"`
	}{w.Value})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"value":"amount"}`
	if string(out) != want {
		t.Errorf("expected %s, got %s", want, string(out))
	}
}

func TestValueEncoding_JSON_MarshalObjectWhenMetadataPresent(t *testing.T) {
	v := &ValueEncoding{Field: "amount", Type: "number", Format: "$,.2f"}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if got["field"] != "amount" || got["type"] != "number" || got["format"] != "$,.2f" {
		t.Errorf("expected object form with field/type/format, got %v", got)
	}
}

func TestValueEncoding_YAML_RoundTripBare(t *testing.T) {
	yamlIn := "value: amount\n"
	var w struct {
		Value *ValueEncoding `yaml:"value"`
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

func TestValueEncoding_YAML_ObjectFormFieldOptional(t *testing.T) {
	yamlBody := "value:\n  type: number\n  format: \"$,.2f\"\n"
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.Value == nil {
		t.Fatal("expected Value to be populated")
	}
	if w.Value.Field != "" {
		t.Errorf("expected Field empty, got %q", w.Value.Field)
	}
	if w.Value.Format != "$,.2f" {
		t.Errorf("expected Format %q, got %q", "$,.2f", w.Value.Format)
	}
}
