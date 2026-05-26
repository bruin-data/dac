package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
)

type validateCall struct {
	connection string
	sql        string
}

type dryRunRecordingBackend struct {
	calls []validateCall
	err   error
}

func (b *dryRunRecordingBackend) Execute(context.Context, string, string) (*query.QueryResult, error) {
	return nil, fmt.Errorf("Execute should not be called when DryRun is available")
}

func (b *dryRunRecordingBackend) DryRun(_ context.Context, connection string, sql string) (*query.DryRunResult, error) {
	b.calls = append(b.calls, validateCall{connection: connection, sql: sql})
	if b.err != nil {
		return nil, b.err
	}
	return &query.DryRunResult{Valid: true}, nil
}

type executeOnlyRecordingBackend struct {
	calls []validateCall
	err   error
}

func (b *executeOnlyRecordingBackend) Execute(_ context.Context, connection string, sql string) (*query.QueryResult, error) {
	b.calls = append(b.calls, validateCall{connection: connection, sql: sql})
	if b.err != nil {
		return nil, b.err
	}
	return &query.QueryResult{}, nil
}

func TestValidateDashboardWithDatabaseDryRunsWidgetQueries(t *testing.T) {
	d := &dashboard.Dashboard{
		Name:       "Sales",
		Connection: "warehouse",
		Queries: map[string]dashboard.Query{
			"total_revenue": {SQL: "SELECT SUM(amount) AS value FROM sales"},
		},
		Rows: []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name:     "Total Revenue",
				Type:     dashboard.WidgetTypeMetric,
				QueryRef: "total_revenue",
				Value:    &dashboard.ValueEncoding{Field: "value"},
			}},
		}},
	}
	backend := &dryRunRecordingBackend{}

	results, err := validateDashboardWithDatabase(context.Background(), backend, d)
	if err != nil {
		t.Fatalf("database validation failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 validation result, got %d", len(results))
	}
	if results[0].label != "Total Revenue" || results[0].connection != "warehouse" || results[0].err != nil {
		t.Fatalf("unexpected result: %+v", results[0])
	}
	if len(backend.calls) != 1 {
		t.Fatalf("expected 1 dry-run call, got %d", len(backend.calls))
	}
	if backend.calls[0].sql != "SELECT SUM(amount) AS value FROM sales" {
		t.Fatalf("unexpected dry-run SQL %q", backend.calls[0].sql)
	}
}

func TestDryRunQueryFallsBackToExplain(t *testing.T) {
	backend := &executeOnlyRecordingBackend{}

	if err := dryRunQuery(context.Background(), backend, "warehouse", "SELECT 1"); err != nil {
		t.Fatalf("dry-run fallback failed: %v", err)
	}
	if len(backend.calls) != 1 {
		t.Fatalf("expected 1 execute call, got %d", len(backend.calls))
	}
	if backend.calls[0].sql != "EXPLAIN SELECT 1" {
		t.Fatalf("expected EXPLAIN fallback, got %q", backend.calls[0].sql)
	}
}

func TestDryRunQueryRejectsPotentiallyMutatingSQL(t *testing.T) {
	backend := &dryRunRecordingBackend{}

	err := dryRunQuery(context.Background(), backend, "warehouse", "DELETE FROM sales")
	if err == nil {
		t.Fatal("expected mutating SQL to fail")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
	if len(backend.calls) != 0 {
		t.Fatalf("expected no backend calls, got %d", len(backend.calls))
	}
}

func TestValidateDryRunSQLSafety(t *testing.T) {
	cases := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{
			name: "select with comments and string literals",
			sql: `-- dashboard query
SELECT 'delete' AS action, created_at FROM sales;`,
		},
		{
			name: "cte select",
			sql:  `WITH totals AS (SELECT SUM(amount) AS value FROM sales) SELECT value FROM totals`,
		},
		{
			name:    "multiple statements",
			sql:     `SELECT 1; SELECT 2`,
			wantErr: true,
		},
		{
			name:    "delete statement",
			sql:     `DELETE FROM sales`,
			wantErr: true,
		},
		{
			name:    "cte with mutating statement",
			sql:     `WITH old_rows AS (SELECT * FROM sales) DELETE FROM sales`,
			wantErr: true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDryRunSQL(tt.sql)
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected SQL to pass, got %v", err)
			}
		})
	}
}

func TestValidateCommandWithDatabaseUsesBruinDryRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "dashboards"), 0o755); err != nil {
		t.Fatalf("create dashboards dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".bruin.yml"), []byte(`default_environment: default
environments:
  default:
    connections:
      duckdb:
        - name: warehouse
          path: test.db
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dashboards", "sales.yml"), []byte(`name: Sales
connection: warehouse
queries:
  total:
    sql: SELECT 1 AS value
rows:
  - widgets:
      - name: Total
        type: metric
        query: total
        value:
          field: value
`), 0o644); err != nil {
		t.Fatalf("write dashboard: %v", err)
	}

	bruinDir := t.TempDir()
	argsPath := filepath.Join(bruinDir, "args.txt")
	bruinPath := filepath.Join(bruinDir, "bruin")
	script := `#!/bin/sh
printf '%s\n' "$@" >> "` + argsPath + `"
printf '{"connectionName":"warehouse","connectionType":"duckdb","query":"SELECT 1 AS value","valid":true}'
`
	if err := os.WriteFile(bruinPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bruin: %v", err)
	}
	t.Setenv("PATH", bruinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	app := NewApp(BuildInfo{Version: "test"})
	output := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"dac", "validate", "--with-database", "--dir", dir}); err != nil {
			t.Fatalf("validate failed: %v", err)
		}
	})
	if !strings.Contains(output, "1 database dry-run query(s) passed") {
		t.Fatalf("expected dry-run summary, got %q", output)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake bruin args: %v", err)
	}
	if !strings.Contains(string(data), "--dry-run") {
		t.Fatalf("expected bruin --dry-run flag, got %q", data)
	}
}

func TestValidateCommandAcceptsPositionalDirectory(t *testing.T) {
	app := NewApp(BuildInfo{Version: "test"})

	output := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{"dac", "validate", filepath.Join("..", "testdata", "dashboards")}); err != nil {
			t.Fatalf("validate failed: %v", err)
		}
	})

	if !strings.Contains(output, "8 dashboard(s) validated successfully") {
		t.Fatalf("expected testdata dashboards to validate, got %q", output)
	}
	if strings.Contains(output, "No dashboard files found in .") {
		t.Fatalf("positional directory was ignored: %q", output)
	}
}
