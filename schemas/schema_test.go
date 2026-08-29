package schemas

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSchemasValidateMinimalDocuments(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Minimal
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "dashboard filter types",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Filter Types
filters:
  - name: as_of_date
    type: date
    default: "2025-01-31"
  - name: min_value
    type: number
    default: 100
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "dashboard vega-lite chart",
			schemaID: DashboardV1ID,
			yaml: `name: Vega-Lite
rows:
  - widgets:
      - name: Layered
        type: chart
        chart: vega-lite
        data:
          columns: [x, y]
          rows: [[A, 1], [B, 2]]
        spec:
          data: { name: dac }
          layer:
            - mark: line
            - mark: point
`,
		},
		{
			name:     "theme",
			schemaID: ThemeV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/theme/v1
name: corporate
tokens:
  background: "#ffffff"
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err != nil {
				t.Fatalf("expected valid %s: %v", tt.name, err)
			}
		})
	}
}

func TestSchemasAllowMissingSchema(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard",
			schemaID: DashboardV1ID,
			yaml: `name: Minimal
rows:
  - widgets:
      - name: One
        type: metric
        sql: SELECT 1 AS value
        value: { field: value }
`,
		},
		{
			name:     "theme",
			schemaID: ThemeV1ID,
			yaml: `name: corporate
tokens:
  background: "#ffffff"
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err != nil {
				t.Fatalf("expected valid %s without schema: %v", tt.name, err)
			}
		})
	}
}

func TestSchemasRejectInvalidDocuments(t *testing.T) {
	cases := []struct {
		name     string
		schemaID string
		yaml     string
	}{
		{
			name:     "dashboard wrong schema",
			schemaID: DashboardV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/dashboard/v2
name: Wrong Schema
rows:
  - widgets:
      - name: One
        type: metric
`,
		},
		{
			name:     "theme missing tokens",
			schemaID: ThemeV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/theme/v1
name: corporate
`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateYAML(tt.schemaID, []byte(tt.yaml)); err == nil {
				t.Fatalf("expected invalid %s", tt.name)
			}
		})
	}
}

func TestSchemasValidateRepositoryYAML(t *testing.T) {
	validateFiles(t, DashboardV1ID,
		"../examples/*/dashboards/*.yml",
		"../examples/*/dashboards/*.yaml",
		"../testdata/dashboards/*.yml",
		"../testdata/dashboards/*.yaml",
		"../testdata/project/dashboards/*.yml",
		"../testdata/project/dashboards/*.yaml",
	)
	validateFiles(t, ThemeV1ID,
		"../testdata/themes/*.yml",
		"../testdata/themes/*.yaml",
	)
}

func validateFiles(t *testing.T, schemaID string, patterns ...string) {
	t.Helper()

	var matched int
	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("bad glob %q: %v", pattern, err)
		}
		for _, file := range files {
			if filepath.Base(file)[0] == '.' {
				continue
			}
			matched++
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			if err := ValidateYAML(schemaID, data); err != nil {
				t.Fatalf("%s does not match %s: %v", file, schemaID, err)
			}
		}
	}
	if matched == 0 {
		t.Fatalf("no files matched for schema %s", schemaID)
	}
}
