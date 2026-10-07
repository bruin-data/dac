package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNoteDimensionRoundTrip(t *testing.T) {
	yamlBody := `
notes:
  - id: rollout
    dimensions:
      - { name: app, type: text, required: true }
      - { name: country, type: select, multiselect: true }
      - { name: day, type: date }
      - { name: priority, type: number }
      - { name: active, type: boolean }
      - { name: comment, type: text }
rows: []
`
	var dashboard Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	encoded, err := json.Marshal(dashboard.Notes[0])
	if err != nil {
		t.Fatalf("marshal note: %v", err)
	}
	want := `{"id":"rollout","dimensions":[{"name":"app","type":"text","required":true},{"name":"country","type":"select","multiselect":true},{"name":"day","type":"date"},{"name":"priority","type":"number"},{"name":"active","type":"boolean"},{"name":"comment","type":"text"}]}`
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
