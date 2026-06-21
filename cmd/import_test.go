package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
)

const metabaseImportFixture = `{
  "name": "Sales Ops",
  "ordered_cards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 3,
    "card": {
      "name": "Orders",
      "display": "scalar",
      "dataset_query": {
        "type": "native",
        "native": {"query": "SELECT COUNT(*) AS order_count FROM orders"}
      },
      "result_metadata": [
        {"name": "order_count", "base_type": "type/Integer"}
      ]
    }
  }]
}`

func TestImportMetabaseCommandWritesDashboardYAML(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "metabase.json")
	if err := os.WriteFile(input, []byte(metabaseImportFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	output := filepath.Join(dir, "dashboards", "sales-ops.yml")

	app := NewApp(BuildInfo{Version: "test"})
	stdout := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{
			"dac", "import", "metabase",
			"--input", input,
			"--output", output,
			"--connection", "warehouse",
		}); err != nil {
			t.Fatalf("import failed: %v", err)
		}
	})
	if !strings.Contains(stdout, "Imported \"Sales Ops\"") {
		t.Fatalf("expected import summary, got %q", stdout)
	}

	d, err := dashboard.LoadFile(output)
	if err != nil {
		t.Fatalf("load generated dashboard: %v", err)
	}
	if d.Connection != "warehouse" {
		t.Fatalf("expected connection to be set, got %q", d.Connection)
	}
	if err := dashboard.Validate(d); err != nil {
		t.Fatalf("generated dashboard should validate: %v", err)
	}
}

func TestImportMetabaseCommandFetchesDashboard(t *testing.T) {
	var gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dashboard/42" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAPIKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(metabaseImportFixture))
	}))
	defer server.Close()

	dir := t.TempDir()
	output := filepath.Join(dir, "dashboard.yml")
	app := NewApp(BuildInfo{Version: "test"})

	if err := app.Run(context.Background(), []string{
		"dac", "import", "metabase",
		"--url", server.URL,
		"--dashboard-id", "42",
		"--api-key", "secret",
		"--output", output,
	}); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if gotAPIKey != "secret" {
		t.Fatalf("expected API key header, got %q", gotAPIKey)
	}
	if _, err := dashboard.LoadFile(output); err != nil {
		t.Fatalf("load generated dashboard: %v", err)
	}
}

func TestImportMetabaseCommandHydratesSourceModelCards(t *testing.T) {
	var fetchedSourceCard bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/dashboard/42":
			_, _ = w.Write([]byte(`{
  "name": "Model Import",
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 6,
    "card": {
      "id": 100,
      "name": "Revenue by Region",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 99,
          "breakout": [["field", {"base-type": "type/Text"}, "region"]],
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      },
      "result_metadata": [
        {"name": "region", "base_type": "type/Text"},
        {"name": "sum", "base_type": "type/Decimal"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["region"],
        "graph.metrics": ["sum"]
      }
    }
  }]
}`))
		case "/api/card/99":
			fetchedSourceCard = true
			_, _ = w.Write([]byte(`{
  "id": 99,
  "name": "Sales Model",
  "type": "model",
  "display": "table",
  "dataset_query": {
    "database": 2,
    "stages": [{
      "native": "SELECT region, revenue FROM sales_summary"
    }]
  }
}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	output := filepath.Join(dir, "dashboard.yml")
	app := NewApp(BuildInfo{Version: "test"})

	if err := app.Run(context.Background(), []string{
		"dac", "import", "metabase",
		"--url", server.URL,
		"--dashboard-id", "42",
		"--api-key", "secret",
		"--output", output,
	}); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if !fetchedSourceCard {
		t.Fatal("expected source model card to be fetched")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "WITH source AS") || !strings.Contains(string(data), "SELECT region, revenue FROM sales_summary") {
		t.Fatalf("expected hydrated model SQL, got:\n%s", string(data))
	}
}

func TestImportMetabaseCommandHydratesPhysicalSourceTables(t *testing.T) {
	var fetchedPhysicalTableAsCard bool
	var fetchedTableMetadata bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/dashboard/42":
			_, _ = w.Write([]byte(`{
  "name": "Physical Table Import",
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 6,
    "card": {
      "id": 100,
      "name": "Orders by Status",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-table": 15,
          "breakout": [["field", {"base-type": "type/Text"}, "status"]],
          "aggregation": [["count"]]
        }]
      },
      "result_metadata": [
        {"name": "status", "base_type": "type/Text"},
        {"name": "count", "base_type": "type/Integer"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["status"],
        "graph.metrics": ["count"]
      }
    }
  }]
}`))
		case "/api/card/15":
			fetchedPhysicalTableAsCard = true
			http.Error(w, "physical tables are not cards", http.StatusInternalServerError)
		case "/api/table/15/query_metadata":
			fetchedTableMetadata = true
			_, _ = w.Write([]byte(`{
  "id": 15,
  "name": "orders",
  "schema": "public",
  "fields": [
    {"id": 201, "name": "status", "base_type": "type/Text"}
  ]
}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	output := filepath.Join(dir, "dashboard.yml")
	app := NewApp(BuildInfo{Version: "test"})

	if err := app.Run(context.Background(), []string{
		"dac", "import", "metabase",
		"--url", server.URL,
		"--dashboard-id", "42",
		"--api-key", "secret",
		"--output", output,
	}); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if fetchedPhysicalTableAsCard {
		t.Fatal("did not expect numeric source-table to be fetched as a card")
	}
	if !fetchedTableMetadata {
		t.Fatal("expected physical source table metadata to be fetched")
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), `FROM "public"."orders"`) || strings.Contains(string(data), "Unsupported Metabase card") {
		t.Fatalf("expected hydrated physical table SQL, got:\n%s", string(data))
	}
}

func TestImportMetabaseCommandWritesSemanticModelFiles(t *testing.T) {
	var fetchedSourceCard bool
	var fetchedMetric bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/dashboard/42":
			_, _ = w.Write([]byte(`{
  "name": "Semantic Model Import",
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 6,
    "card": {
      "id": 100,
      "name": "Official Revenue by Region",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 99,
          "breakout": [["field", {"base-type": "type/Text"}, "region"]],
          "aggregation": [["metric", 200]]
        }]
      }
    }
  }]
}`))
		case "/api/card/99":
			fetchedSourceCard = true
			_, _ = w.Write([]byte(`{
  "id": 99,
  "name": "Sales Model",
  "type": "model",
  "dataset_query": {
    "database": 2,
    "stages": [{
      "native": "SELECT region, revenue FROM sales_summary"
    }]
  },
  "result_metadata": [
    {"name": "region", "base_type": "type/Text"},
    {"name": "revenue", "base_type": "type/Decimal"}
  ]
}`))
		case "/api/metric/200":
			fetchedMetric = true
			_, _ = w.Write([]byte(`{
  "id": 200,
  "name": "Official Revenue",
  "type": "metric",
  "dataset_query": {
    "database": 2,
    "stages": [{
      "source-card": 99,
      "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
    }]
  }
}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	output := filepath.Join(dir, "dashboards", "semantic-model-import.yml")
	app := NewApp(BuildInfo{Version: "test"})

	stdout := captureStdout(t, func() {
		if err := app.Run(context.Background(), []string{
			"dac", "import", "metabase",
			"--url", server.URL,
			"--dashboard-id", "42",
			"--api-key", "secret",
			"--output", output,
			"--connection", "warehouse",
			"--semantic",
		}); err != nil {
			t.Fatalf("import failed: %v", err)
		}
	})
	if !fetchedSourceCard || !fetchedMetric {
		t.Fatalf("expected source model and metric to be fetched, source=%v metric=%v", fetchedSourceCard, fetchedMetric)
	}
	if !strings.Contains(stdout, "1 semantic model file") {
		t.Fatalf("expected semantic import summary, got %q", stdout)
	}

	modelPath := filepath.Join(dir, "semantic", "sales_model.yml")
	modelData, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatalf("read generated semantic model: %v", err)
	}
	if !strings.Contains(string(modelData), "name: sales_model") || !strings.Contains(string(modelData), "name: official_revenue") {
		t.Fatalf("unexpected semantic model YAML:\n%s", string(modelData))
	}

	d, err := dashboard.LoadFile(output)
	if err != nil {
		t.Fatalf("load generated dashboard: %v", err)
	}
	if err := dashboard.Validate(d); err != nil {
		t.Fatalf("generated semantic dashboard should validate: %v", err)
	}
}

func TestImportMetabaseCommandRejectsSemanticStdout(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "metabase.json")
	if err := os.WriteFile(input, []byte(metabaseImportFixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	app := NewApp(BuildInfo{Version: "test"})
	err := app.Run(context.Background(), []string{
		"dac", "import", "metabase",
		"--input", input,
		"--output", "-",
		"--semantic",
	})
	if err == nil {
		t.Fatal("expected semantic stdout import to fail")
	}
	if !strings.Contains(err.Error(), "--semantic cannot be used with --output -") {
		t.Fatalf("unexpected error: %v", err)
	}
}
