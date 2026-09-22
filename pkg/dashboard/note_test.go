package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNoteDimensionOptionsRoundTrip(t *testing.T) {
	yamlBody := `
notes:
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

	dimensions := dashboard.Notes[0].Dimensions
	if !dimensions[0].Required {
		t.Fatal("expected app to preserve required: true")
	}
	if !dimensions[1].Multiselect {
		t.Fatal("expected country to preserve multiselect: true")
	}
	if dimensions[1].Required {
		t.Fatal("expected omitted required to default to false")
	}

	encoded, err := json.Marshal(dashboard.Notes[0])
	if err != nil {
		t.Fatalf("marshal note: %v", err)
	}
	want := `{"id":"rollout","dimensions":[{"name":"app","required":true},{"name":"country","multiselect":true}]}`
	if string(encoded) != want {
		t.Fatalf("unexpected note JSON:\n got: %s\nwant: %s", encoded, want)
	}
}

func TestNoteWithEmptyDimensionsRoundTrip(t *testing.T) {
	yamlBody := `
notes:
  - id: dashboard_context
    dimensions: []
rows: []
`

	var dashboard Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	if len(dashboard.Notes) != 1 || dashboard.Notes[0].Dimensions == nil || len(dashboard.Notes[0].Dimensions) != 0 {
		t.Fatalf("expected a dimensionless note, got %#v", dashboard.Notes)
	}

	yamlEncoded, err := yaml.Marshal(dashboard.Notes[0])
	if err != nil {
		t.Fatalf("marshal note as YAML: %v", err)
	}
	if !strings.Contains(string(yamlEncoded), "dimensions: []") {
		t.Fatalf("empty dimensions were not preserved in YAML: %s", yamlEncoded)
	}

	encoded, err := json.Marshal(dashboard.Notes[0])
	if err != nil {
		t.Fatalf("marshal note: %v", err)
	}
	if string(encoded) != `{"id":"dashboard_context","dimensions":[]}` {
		t.Fatalf("unexpected note JSON: %s", encoded)
	}
}
