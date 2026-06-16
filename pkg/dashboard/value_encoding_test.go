package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValueEncoding_YAML_BareScalarRejected(t *testing.T) {
	yamlBody := `value: amount`
	var w Widget
	err := yaml.Unmarshal([]byte(yamlBody), &w)
	if err == nil {
		t.Fatal("expected error for bare scalar value, got nil")
	}
	if !strings.Contains(err.Error(), "field key") {
		t.Errorf("expected guidance toward object form, got: %v", err)
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
	if w.ValueField() != "total_revenue" {
		t.Errorf("ValueField() = %q, want %q", w.ValueField(), "total_revenue")
	}
}

func TestValueEncoding_JSON_BareScalarRejected(t *testing.T) {
	body := []byte(`{"value": "amount"}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err == nil {
		t.Fatal("expected error for bare scalar value, got nil")
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

func TestValueEncoding_JSON_MarshalAlwaysObject(t *testing.T) {
	w := Widget{Value: &ValueEncoding{Field: "amount"}}
	out, err := json.Marshal(struct {
		Value *ValueEncoding `json:"value"`
	}{w.Value})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"value":{"field":"amount"}}`
	if string(out) != want {
		t.Errorf("expected %s, got %s", want, string(out))
	}
}

func TestValueEncoding_JSON_MarshalIncludesMetadata(t *testing.T) {
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

func TestValueEncoding_YAML_RoundTripObject(t *testing.T) {
	yamlIn := "value:\n    field: amount\n"
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

func TestValueEncoding_YAML_ObjectFormRequiresField(t *testing.T) {
	cases := map[string]string{
		"empty object":  `value: {}`,
		"metadata only": "value:\n  type: number\n",
		"explicit null": "value:\n  field: null\n  type: number\n",
		"empty string":  "value:\n  field: \"\"\n  type: number\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var w Widget
			err := yaml.Unmarshal([]byte(body), &w)
			if err == nil {
				t.Fatalf("expected error for input %q, got nil", body)
			}
		})
	}
}

func TestValueEncoding_JSON_ObjectFormRequiresField(t *testing.T) {
	cases := map[string][]byte{
		"empty object":  []byte(`{"value": {}}`),
		"metadata only": []byte(`{"value": {"type": "number"}}`),
		"explicit null": []byte(`{"value": {"field": null, "type": "number"}}`),
		"empty string":  []byte(`{"value": {"field": "", "type": "number"}}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var w Widget
			err := json.Unmarshal(body, &w)
			if err == nil {
				t.Fatalf("expected error for input %s, got nil", string(body))
			}
		})
	}
}
