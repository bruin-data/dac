package dashboard

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWidgetID_YAML(t *testing.T) {
	yamlBody := `
name: w
type: metric
id: kpi_rev
sql: SELECT 1 AS value
column: value
`
	var w Widget
	if err := yaml.Unmarshal([]byte(yamlBody), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.ID != "kpi_rev" {
		t.Errorf("expected ID = %q, got %q", "kpi_rev", w.ID)
	}
}

func TestWidgetID_JSON(t *testing.T) {
	body := []byte(`{"name":"w","type":"metric","id":"kpi_rev","sql":"SELECT 1","column":"value"}`)
	var w Widget
	if err := json.Unmarshal(body, &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.ID != "kpi_rev" {
		t.Errorf("expected ID = %q, got %q", "kpi_rev", w.ID)
	}
}

func TestWidgetID_OmittedWhenEmpty(t *testing.T) {
	w := Widget{Name: "w", Type: "text", Content: "hi"}
	out, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(out); got == "" || containsKey(got, "id") {
		t.Errorf("expected no id key when ID empty, got: %s", got)
	}
}

func TestWidgetID_Roundtrip(t *testing.T) {
	w := Widget{ID: "kpi_rev", Name: "w", Type: "metric"}
	out, err := yaml.Marshal(&w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Widget
	if err := yaml.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if back.ID != "kpi_rev" {
		t.Errorf("roundtrip lost ID: got %q", back.ID)
	}
}

func TestWidgetID_OptionalNotRequired(t *testing.T) {
	d := &Dashboard{
		Name: "t",
		Rows: []Row{{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}}},
	}
	if err := Validate(d); err != nil {
		t.Fatalf("expected no error for widget without id, got: %v", err)
	}
}

func containsKey(jsonStr, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
