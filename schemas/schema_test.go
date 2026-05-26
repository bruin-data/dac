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
        value:
          field: value
`,
		},
		{
			name:     "semantic model",
			schemaID: SemanticModelV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/semantic-model/v1
name: sales
source:
  table: sales
metrics:
  - name: revenue
    expression: sum(amount)
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
        value:
          field: value
`,
		},
		{
			name:     "semantic model",
			schemaID: SemanticModelV1ID,
			yaml: `name: sales
source:
  table: sales
metrics:
  - name: revenue
    expression: sum(amount)
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
			name:     "semantic model extra field",
			schemaID: SemanticModelV1ID,
			yaml: `schema: https://getbruin.com/schemas/dac/semantic-model/v1
name: sales
unknown: true
source:
  table: sales
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
	validateFiles(t, SemanticModelV1ID,
		"../examples/*/semantic/*.yml",
		"../examples/*/semantic/*.yaml",
		"../testdata/semantic/*.yml",
		"../testdata/semantic/*.yaml",
		"../testdata/project/semantic/*.yml",
		"../testdata/project/semantic/*.yaml",
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
