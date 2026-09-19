package dashboard

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNoteDimensionOptionsRoundTrip(t *testing.T) {
	yamlBody := `
notes:
  - id: rollout
    dimensions:
      - { name: app, optional: false }
      - { name: country, multiselect: true }
rows: []
`

	var dashboard Dashboard
	if err := yaml.Unmarshal([]byte(yamlBody), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}

	dimensions := dashboard.Notes[0].Dimensions
	if dimensions[0].Optional == nil || *dimensions[0].Optional {
		t.Fatal("expected app to preserve optional: false")
	}
	if !dimensions[1].Multiselect {
		t.Fatal("expected country to preserve multiselect: true")
	}
	if dimensions[1].Optional != nil {
		t.Fatal("expected omitted optional to remain unset")
	}

	encoded, err := json.Marshal(dashboard.Notes[0])
	if err != nil {
		t.Fatalf("marshal note: %v", err)
	}
	want := `{"id":"rollout","dimensions":[{"name":"app","optional":false},{"name":"country","multiselect":true}]}`
	if string(encoded) != want {
		t.Fatalf("unexpected note JSON:\n got: %s\nwant: %s", encoded, want)
	}
}
