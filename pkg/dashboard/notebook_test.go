package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNotebookDimensionOptionsRoundTrip(t *testing.T) {
	yamlBody := `
notebooks:
  - id: rollout
    dimensions:
      - { name: app, required: true }
      - { name: country, multiselect: true }
rows: []
`

	var dashboard Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}

	dimensions := dashboard.Notebooks[0].Dimensions
	if !dimensions[0].Required {
		t.Fatal("expected app to preserve required: true")
	}
	if !dimensions[1].Multiselect {
		t.Fatal("expected country to preserve multiselect: true")
	}
	if dimensions[1].Required {
		t.Fatal("expected omitted required to default to false")
	}

	encoded, err := json.Marshal(dashboard.Notebooks[0])
	if err != nil {
		t.Fatalf("marshal notebook: %v", err)
	}
	want := `{"id":"rollout","dimensions":[{"name":"app","required":true},{"name":"country","multiselect":true}]}`
	if string(encoded) != want {
		t.Fatalf("unexpected notebook JSON:\n got: %s\nwant: %s", encoded, want)
	}
}

func TestNotebookWithEmptyDimensionsRoundTrip(t *testing.T) {
	yamlBody := `
notebooks:
  - id: dashboard_context
    dimensions: []
rows: []
`

	var dashboard Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	if len(dashboard.Notebooks) != 1 || dashboard.Notebooks[0].Dimensions == nil || len(dashboard.Notebooks[0].Dimensions) != 0 {
		t.Fatalf("expected a dimensionless notebook, got %#v", dashboard.Notebooks)
	}

	yamlEncoded, err := yaml.Marshal(dashboard.Notebooks[0])
	if err != nil {
		t.Fatalf("marshal notebook as YAML: %v", err)
	}
	if !strings.Contains(string(yamlEncoded), "dimensions: []") {
		t.Fatalf("empty dimensions were not preserved in YAML: %s", yamlEncoded)
	}

	encoded, err := json.Marshal(dashboard.Notebooks[0])
	if err != nil {
		t.Fatalf("marshal notebook: %v", err)
	}
	if string(encoded) != `{"id":"dashboard_context","dimensions":[]}` {
		t.Fatalf("unexpected notebook JSON: %s", encoded)
	}
}
