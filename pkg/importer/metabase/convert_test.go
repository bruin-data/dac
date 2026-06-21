package metabase

import (
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
)

func TestConvertNativeSQLDashboard(t *testing.T) {
	input := []byte(`{
  "name": "Executive Sales",
  "description": "Imported from Metabase",
  "parameters": [
    {"id": "region_param", "slug": "region", "name": "Region", "type": "category", "default": "All", "values": ["All", "EU", "US"]}
  ],
  "ordered_cards": [
    {
      "row": 0,
      "col": 0,
      "size_x": 12,
      "size_y": 4,
      "card_id": 10,
      "parameter_mappings": [{
        "parameter_id": "region_param",
        "card_id": 10,
        "target": ["variable", ["template-tag", "region"]]
      }],
      "card": {
        "id": 10,
        "name": "Revenue Trend",
        "display": "line",
        "dataset_query": {
          "type": "native",
          "native": {
            "query": "SELECT month, revenue FROM sales WHERE region = '{{region}}' [[AND created_at >= '{{start_date}}']]",
            "template_tags": {
              "region": {"name": "region", "type": "text"},
              "start_date": {"name": "start_date", "type": "date"}
            }
          }
        },
        "result_metadata": [
          {"name": "month", "base_type": "type/Date"},
          {"name": "revenue", "base_type": "type/Float"}
        ],
        "visualization_settings": {
          "graph.dimensions": ["month"],
          "graph.metrics": ["revenue"]
        }
      }
    },
    {
      "row": 0,
      "col": 12,
      "size_x": 12,
      "size_y": 3,
      "card": {
        "name": "Order Count",
        "display": "scalar",
        "dataset_query": {
          "type": "native",
          "native": {"query": "SELECT COUNT(*) AS order_count FROM orders"}
        },
        "result_metadata": [
          {"name": "order_count", "base_type": "type/Integer"}
        ]
      }
    }
  ]
}`)

	d, report, err := Convert(input, Options{Connection: "warehouse"})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if err := dashboard.Validate(d); err != nil {
		t.Fatalf("generated dashboard should validate: %v", err)
	}
	if d.Name != "Executive Sales" || d.Connection != "warehouse" {
		t.Fatalf("unexpected dashboard metadata: %+v", d)
	}
	if len(d.Filters) != 1 {
		t.Fatalf("expected only the dashboard filter, got %d", len(d.Filters))
	}
	if d.Rows[0].Widgets[0].Chart != "line" {
		t.Fatalf("expected line chart, got %+v", d.Rows[0].Widgets[0])
	}
	if got := d.Rows[0].Widgets[0].SQL; !strings.Contains(got, "{{ filters.region }}") || strings.Contains(got, "start_date") {
		t.Fatalf("expected only the mapped SQL template variable to use a DAC filter, got %q", got)
	}
	if strings.Contains(d.Rows[0].Widgets[0].SQL, "[[") {
		t.Fatalf("expected Metabase optional markers to be removed, got %q", d.Rows[0].Widgets[0].SQL)
	}
	if d.Rows[0].Widgets[1].Type != dashboard.WidgetTypeMetric {
		t.Fatalf("expected scalar card to become metric, got %+v", d.Rows[0].Widgets[1])
	}
	if d.Rows[0].Height != nil {
		t.Fatalf("expected Metabase size_y not to set DAC row height, got %+v", d.Rows[0].Height)
	}
	if report.WidgetCount != 2 || report.SQLWidgetCount != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected warning about optional SQL clause conversion")
	}
}

func TestConvertUnquotedMetabaseTemplateVariablesQuotesDACFilters(t *testing.T) {
	input := []byte(`{
  "name": "Unquoted Variables",
  "parameters": [
    {"id": "region_param", "slug": "region", "name": "Region", "type": "category"},
    {"id": "start_param", "slug": "start_date", "name": "Start Date", "type": "date"},
    {"id": "orders_param", "slug": "min_orders", "name": "Minimum Orders", "type": "number"}
  ],
  "ordered_cards": [{
    "card_id": 20,
    "parameter_mappings": [
      {"parameter_id": "region_param", "card_id": 20, "target": ["variable", ["template-tag", "region"]]},
      {"parameter_id": "start_param", "card_id": 20, "target": ["variable", ["template-tag", "start_date"]]},
      {"parameter_id": "orders_param", "card_id": 20, "target": ["variable", ["template-tag", "min_orders"]]}
    ],
    "card": {
      "id": 20,
      "name": "Revenue",
      "display": "scalar",
      "dataset_query": {
        "type": "native",
        "native": {
          "query": "SELECT SUM(revenue) AS total_revenue FROM sales_summary WHERE region = {{region}} AND order_date >= {{start_date}} AND orders >= {{min_orders}}",
          "template_tags": {
            "region": {"name": "region", "type": "text"},
            "start_date": {"name": "start_date", "type": "date"},
            "min_orders": {"name": "min_orders", "type": "number"}
          }
        }
      },
      "result_metadata": [
        {"name": "total_revenue", "base_type": "type/Decimal"}
      ]
    }
  }]
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	sql := d.Rows[0].Widgets[0].SQL
	for _, want := range []string{
		"region = '{{ filters.region }}'",
		"order_date >= '{{ filters.start_date }}'",
		"orders >= {{ filters.min_orders }}",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q, got %q", want, sql)
		}
	}
}

func TestConvertMetabaseTemplateVariableInsideStringLiteral(t *testing.T) {
	input := []byte(`{
  "name": "String Pattern Variables",
  "parameters": [
    {"id": "search_param", "slug": "search", "name": "Search", "type": "text"}
  ],
  "ordered_cards": [{
    "card_id": 22,
    "parameter_mappings": [{
      "parameter_id": "search_param",
      "card_id": 22,
      "target": ["variable", ["template-tag", "search"]]
    }],
    "card": {
      "id": 22,
      "name": "Customer Search",
      "display": "table",
      "dataset_query": {
        "type": "native",
        "native": {
          "query": "SELECT customer_name FROM customers WHERE customer_name LIKE '%{{search}}%'",
          "template_tags": {
            "search": {"name": "search", "type": "text"}
          }
        }
      },
      "result_metadata": [
        {"name": "customer_name", "base_type": "type/Text"}
      ]
    }
  }]
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	sql := d.Rows[0].Widgets[0].SQL
	if !strings.Contains(sql, "LIKE '%{{ filters.search }}%'") {
		t.Fatalf("expected embedded string filter to stay inside the existing literal, got %q", sql)
	}
	if strings.Contains(sql, "'{{ filters.search }}'") {
		t.Fatalf("expected no extra quotes around embedded string filter, got %q", sql)
	}
}

func TestConvertNativeSQLTemplateTagsWithoutDashboardMappingUseDefaults(t *testing.T) {
	input := []byte(`{
  "name": "Native Defaults",
  "parameters": [
    {"id": "region_param", "slug": "region", "name": "Region", "type": "category", "default": "Europe"}
  ],
  "ordered_cards": [{
    "card": {
      "id": 21,
      "name": "Margin",
      "display": "bar",
      "dataset_query": {
        "type": "native",
        "native": {
          "query": "SELECT plan, SUM(revenue) AS revenue FROM revenue_model WHERE 1=1 [[AND region = {{region}}]] [[AND order_date >= {{start_date}}]] [[AND order_date <= {{end_date}}]] [[AND channel = {{channel}}]] GROUP BY 1",
          "template_tags": {
            "region": {"name": "region", "type": "text", "default": "North America"},
            "start_date": {"name": "start_date", "type": "date", "default": "2026-03-01"},
            "end_date": {"name": "end_date", "type": "date", "default": "2026-06-15"},
            "channel": {"name": "channel", "type": "text"}
          }
        }
      },
      "result_metadata": [
        {"name": "plan", "base_type": "type/Text"},
        {"name": "revenue", "base_type": "type/Decimal"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["plan"],
        "graph.metrics": ["revenue"]
      }
    }
  }]
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if len(d.Filters) != 1 || d.Filters[0].Name != "region" {
		t.Fatalf("expected only the Metabase dashboard parameter as a DAC filter, got %+v", d.Filters)
	}
	sql := d.Rows[0].Widgets[0].SQL
	for _, want := range []string{
		"region = 'North America'",
		"order_date >= '2026-03-01'",
		"order_date <= '2026-06-15'",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain default literal %q, got %q", want, sql)
		}
	}
	for _, notWant := range []string{"filters.start_date", "filters.end_date", "filters.region", "channel ="} {
		if strings.Contains(sql, notWant) {
			t.Fatalf("expected SQL not to contain %q, got %q", notWant, sql)
		}
	}
}

func TestConvertMetabaseDateParameterDefaultsValidate(t *testing.T) {
	input := []byte(`{
  "name": "Date Defaults",
  "parameters": [
    {"id": "range_param", "slug": "created_range", "name": "Created Range", "type": "date/all-options", "default": "past30days"},
    {"id": "fixed_range_param", "slug": "fixed_range", "name": "Fixed Range", "type": "date/range", "default": "2026-01-01~2026-01-31"},
    {"id": "literal_range_param", "slug": "literal_range", "name": "Literal Range", "type": "date/all-options", "default": "2026-02-15"},
    {"id": "unknown_range_param", "slug": "unknown_range", "name": "Unknown Range", "type": "date/all-options", "default": "previous-week"},
    {"id": "single_param", "slug": "as_of_date", "name": "As Of Date", "type": "date/single", "default": "today"}
  ],
  "ordered_cards": [{
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
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if len(d.Filters) != 5 {
		t.Fatalf("expected five filters, got %+v", d.Filters)
	}
	if d.Filters[0].Type != "date-range" || d.Filters[0].Default != "last_30_days" {
		t.Fatalf("expected range default to stay a date-range preset, got %+v", d.Filters[0])
	}
	if d.Filters[1].Type != "date-range" {
		t.Fatalf("expected fixed range filter to be date-range, got %+v", d.Filters[1])
	}
	if got, ok := d.Filters[1].Default.(map[string]any); !ok || got["start"] != "2026-01-01" || got["end"] != "2026-01-31" {
		t.Fatalf("expected fixed range default map, got %+v", d.Filters[1].Default)
	}
	if d.Filters[2].Type != "date-range" {
		t.Fatalf("expected literal range filter to be date-range, got %+v", d.Filters[2])
	}
	if got, ok := d.Filters[2].Default.(map[string]any); !ok || got["start"] != "2026-02-15" || got["end"] != "2026-02-15" {
		t.Fatalf("expected literal range default map, got %+v", d.Filters[2].Default)
	}
	if d.Filters[3].Type != "date-range" || d.Filters[3].Default != nil {
		t.Fatalf("expected unknown range default to be omitted, got %+v", d.Filters[3])
	}
	if d.Filters[4].Type != "date" || d.Filters[4].Default != "TODAY" {
		t.Fatalf("expected single-date default to use a DAC date expression, got %+v", d.Filters[4])
	}
	if err := dashboard.Validate(d); err != nil {
		t.Fatalf("generated dashboard should validate: %v", err)
	}
}

func TestConvertUnsupportedMBQLCreatesPlaceholder(t *testing.T) {
	input := []byte(`{
  "name": "Query Builder Dashboard",
  "ordered_cards": [{
    "card": {
      "name": "Built with GUI",
      "display": "bar",
      "dataset_query": {"type": "query", "query": {"source-table": 1}}
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeText {
		t.Fatalf("expected placeholder text widget, got %+v", w)
	}
	if report.PlaceholderCount != 1 || len(report.Warnings) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertPhysicalTableMBQLDetailRows(t *testing.T) {
	input := []byte(`{
  "name": "Physical Table Dashboard",
  "x-dac-metabase-tables": {
    "15": {
      "id": 15,
      "name": "orders",
      "schema": "public",
      "fields": [
        {"id": 112, "name": "order_id", "base_type": "type/Integer"},
        {"id": 117, "name": "region", "base_type": "type/Text"},
        {"id": 129, "name": "net_revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "ordered_cards": [{
    "card": {
      "name": "Raw Orders",
      "display": "bar",
      "dataset_query": {
        "type": "query",
        "query": {"source-table": 15}
      }
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeTable {
		t.Fatalf("expected physical table MBQL without aggregations to become table, got %+v", w)
	}
	for _, want := range []string{
		`SELECT *`,
		`FROM "public"."orders"`,
		`LIMIT 2000`,
	} {
		if !strings.Contains(w.SQL, want) {
			t.Fatalf("expected SQL to contain %q, got:\n%s", want, w.SQL)
		}
	}
	if report.SQLWidgetCount != 1 || report.PlaceholderCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertPhysicalTableMBQLAggregationWithFieldIDs(t *testing.T) {
	input := []byte(`{
  "name": "Physical Table Dashboard",
  "x-dac-metabase-tables": {
    "15": {
      "id": 15,
      "name": "orders",
      "schema": "public",
      "fields": [
        {"id": 115, "name": "order_date", "base_type": "type/Date"},
        {"id": 117, "name": "region", "base_type": "type/Text"},
        {"id": 129, "name": "net_revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "ordered_cards": [{
    "card": {
      "name": "Orders by Region",
      "display": "bar",
      "dataset_query": {
        "type": "query",
        "query": {
          "source-table": 15,
          "breakout": [["field", 117, null]],
          "aggregation": [["sum", ["field", 129, null]]],
          "filter": [">=", ["field", 115, null], "2026-01-01"],
          "order-by": [["desc", ["aggregation", 0]]],
          "limit": 10
        }
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
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeChart || w.Chart != "bar" {
		t.Fatalf("expected bar chart, got %+v", w)
	}
	for _, want := range []string{
		`SELECT "region" AS "region", SUM("net_revenue") AS "sum"`,
		`FROM "public"."orders"`,
		`"order_date" >= '2026-01-01'`,
		`GROUP BY 1`,
		`ORDER BY "sum" DESC`,
		`LIMIT 10`,
	} {
		if !strings.Contains(w.SQL, want) {
			t.Fatalf("expected SQL to contain %q, got:\n%s", want, w.SQL)
		}
	}
	if report.SQLWidgetCount != 1 || report.PlaceholderCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertPhysicalTableUnsupportedMBQLFilterCreatesPlaceholder(t *testing.T) {
	input := []byte(`{
  "name": "Unsupported Filter Dashboard",
  "x-dac-metabase-tables": {
    "15": {
      "id": 15,
      "name": "orders",
      "schema": "public",
      "fields": [
        {"id": 117, "name": "region", "base_type": "type/Text"}
      ]
    }
  },
  "ordered_cards": [{
    "card": {
      "name": "Contains Region",
      "display": "table",
      "dataset_query": {
        "type": "query",
        "query": {
          "source-table": 15,
          "filter": ["contains", ["field", 117, null], "EU"]
        }
      },
      "result_metadata": [
        {"name": "region", "base_type": "type/Text"}
      ]
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeText {
		t.Fatalf("expected unsupported filter to become placeholder, got %+v", w)
	}
	if report.SQLWidgetCount != 0 || report.PlaceholderCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !containsWarning(report.Warnings, "unsupported MBQL filter") {
		t.Fatalf("expected unsupported filter warning, got %+v", report.Warnings)
	}
}

func TestConvertPhysicalTableUnsupportedMBQLAggregationCreatesPlaceholder(t *testing.T) {
	input := []byte(`{
  "name": "Unsupported Aggregation Dashboard",
  "x-dac-metabase-tables": {
    "15": {
      "id": 15,
      "name": "orders",
      "schema": "public",
      "fields": [
        {"id": 117, "name": "region", "base_type": "type/Text"},
        {"id": 129, "name": "net_revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "ordered_cards": [{
    "card": {
      "name": "Median Revenue",
      "display": "bar",
      "dataset_query": {
        "type": "query",
        "query": {
          "source-table": 15,
          "breakout": [["field", 117, null]],
          "aggregation": [["median", ["field", 129, null]]]
        }
      },
      "result_metadata": [
        {"name": "region", "base_type": "type/Text"},
        {"name": "median", "base_type": "type/Decimal"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["region"],
        "graph.metrics": ["median"]
      }
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeText {
		t.Fatalf("expected unsupported aggregation to become placeholder, got %+v", w)
	}
	if report.SQLWidgetCount != 0 || report.PlaceholderCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !containsWarning(report.Warnings, "unsupported MBQL aggregation") {
		t.Fatalf("expected unsupported aggregation warning, got %+v", report.Warnings)
	}
}

func TestConvertPhysicalTableTemporalBreakoutCreatesPlaceholder(t *testing.T) {
	input := []byte(`{
  "name": "Temporal Breakout Dashboard",
  "x-dac-metabase-tables": {
    "15": {
      "id": 15,
      "name": "orders",
      "schema": "public",
      "fields": [
        {"id": 115, "name": "order_date", "base_type": "type/Date"},
        {"id": 129, "name": "net_revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "ordered_cards": [{
    "card": {
      "name": "Monthly Revenue",
      "display": "bar",
      "dataset_query": {
        "type": "query",
        "query": {
          "source-table": 15,
          "breakout": [["field", 115, {"temporal-unit": "month"}]],
          "aggregation": [["sum", ["field", 129, null]]]
        }
      },
      "result_metadata": [
        {"name": "order_date", "base_type": "type/Date"},
        {"name": "sum", "base_type": "type/Decimal"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["order_date"],
        "graph.metrics": ["sum"]
      }
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeText {
		t.Fatalf("expected temporal breakout to become placeholder, got %+v", w)
	}
	if report.SQLWidgetCount != 0 || report.PlaceholderCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if !containsWarning(report.Warnings, "unsupported MBQL breakout") {
		t.Fatalf("expected unsupported breakout warning, got %+v", report.Warnings)
	}
}

func TestConvertMetabaseStagedNativeQuery(t *testing.T) {
	input := []byte(`{
  "name": "Staged Native Dashboard",
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 4,
    "card": {
      "name": "Revenue",
      "display": "scalar",
      "query_type": "native",
      "dataset_query": {
        "lib/type": "mbql/query",
        "database": 2,
        "stages": [{
          "lib/type": "mbql.stage/native",
          "native": "SELECT ROUND(SUM(revenue), 2) AS total_revenue FROM sales_summary"
        }]
      },
      "result_metadata": [
        {"name": "total_revenue", "base_type": "type/Decimal"}
      ]
    }
  }]
}`)

	d, report, err := Convert(input, Options{Connection: "warehouse"})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeMetric {
		t.Fatalf("expected metric widget, got %+v", w)
	}
	if !strings.Contains(w.SQL, "SUM(revenue)") {
		t.Fatalf("expected SQL from staged native query, got %q", w.SQL)
	}
	if report.SQLWidgetCount != 1 || report.PlaceholderCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertNativeScalarInfersValueFieldFromSQLAlias(t *testing.T) {
	input := []byte(`{
  "name": "Native Dashboard",
  "dashcards": [{
    "card": {
      "name": "Revenue",
      "display": "scalar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT ROUND(SUM(revenue), 2) AS total_revenue FROM sales_summary"
        }]
      },
      "result_metadata": null
    }
  }]
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Value == nil || w.Value.Field != "total_revenue" {
		t.Fatalf("expected value field from SQL alias, got %+v", w.Value)
	}
	if w.Value.Type != "number" {
		t.Fatalf("expected inferred numeric value type, got %+v", w.Value)
	}
}

func TestConvertModelBackedQuestionWithDashboardFilter(t *testing.T) {
	input := []byte(`{
  "name": "Model Dashboard",
  "parameters": [{
    "id": "region_param",
    "slug": "region",
    "name": "Region",
    "type": "category",
    "default": "Europe",
    "values_source_config": {"values": ["Europe", "APAC"]}
  }],
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "display": "table",
      "dataset_query": {
        "lib/type": "mbql/query",
        "database": 2,
        "stages": [{
          "lib/type": "mbql.stage/native",
          "native": "SELECT order_date, region, revenue FROM sales_summary;"
        }]
      }
    }
  },
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 6,
    "parameter_mappings": [{
      "parameter_id": "region_param",
      "card_id": 47,
      "target": ["dimension", ["field", {"base-type": "type/Text"}, "region"]]
    }],
    "card": {
      "id": 47,
      "name": "Revenue by Region",
      "display": "bar",
      "query_type": "query",
      "dataset_query": {
        "lib/type": "mbql/query",
        "database": 2,
        "stages": [{
          "lib/type": "mbql.stage/mbql",
          "source-card": 46,
          "breakout": [["field", {"base-type": "type/Text"}, "region"]],
          "aggregation": [["sum", {"lib/uuid": "abc"}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      },
      "result_metadata": [
        {"name": "region", "base_type": "type/Text", "source": "breakout"},
        {"name": "sum", "base_type": "type/Decimal", "source": "aggregation"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["region"],
        "graph.metrics": ["sum"]
      }
    }
  }]
}`)

	d, report, err := Convert(input, Options{Connection: "warehouse"})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if len(d.Filters) != 1 {
		t.Fatalf("expected one dashboard filter, got %d", len(d.Filters))
	}
	if d.Filters[0].Name != "region" || d.Filters[0].Type != "select" {
		t.Fatalf("unexpected filter: %+v", d.Filters[0])
	}
	w := d.Rows[0].Widgets[0]
	if w.Type != dashboard.WidgetTypeChart || w.Chart != "bar" {
		t.Fatalf("expected bar chart, got %+v", w)
	}
	for _, want := range []string{
		"WITH source AS",
		"SELECT order_date, region, revenue FROM sales_summary",
		`SUM("revenue") AS "sum"`,
		`"region" = '{{ filters.region }}'`,
		"GROUP BY 1",
	} {
		if !strings.Contains(w.SQL, want) {
			t.Fatalf("expected generated SQL to contain %q, got:\n%s", want, w.SQL)
		}
	}
	if strings.Contains(w.SQL, "sales_summary;\n)") {
		t.Fatalf("expected trailing source SQL terminator to be stripped, got:\n%s", w.SQL)
	}
	if report.SQLWidgetCount != 1 || report.PlaceholderCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertModelBackedQuestionWithSingleDateDashboardFilter(t *testing.T) {
	input := []byte(`{
  "name": "Model Date Dashboard",
  "parameters": [{
    "id": "as_of_param",
    "slug": "as_of_date",
    "name": "As Of Date",
    "type": "date/single",
    "default": "today"
  }],
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT order_date, revenue FROM sales_summary"
        }]
      }
    }
  },
  "dashcards": [{
    "parameter_mappings": [{
      "parameter_id": "as_of_param",
      "card_id": 47,
      "target": ["dimension", ["field", {"base-type": "type/Date"}, "order_date"]]
    }],
    "card": {
      "id": 47,
      "name": "Revenue As Of Date",
      "display": "scalar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      },
      "result_metadata": [
        {"name": "sum", "base_type": "type/Decimal"}
      ]
    }
  }]
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	sql := d.Rows[0].Widgets[0].SQL
	if !strings.Contains(sql, `"order_date" = '{{ filters.as_of_date }}'`) {
		t.Fatalf("expected single-date dashboard mapping to use equality, got:\n%s", sql)
	}
	if strings.Contains(sql, `"order_date" >=`) {
		t.Fatalf("expected single-date dashboard mapping not to widen to >=, got:\n%s", sql)
	}
}

func TestConvertModelBackedQuestionIgnoresUnscopedDashboardFilterMapping(t *testing.T) {
	input := []byte(`{
  "name": "Model Dashboard",
  "parameters": [{
    "id": "region_param",
    "slug": "region",
    "name": "Region",
    "type": "category",
    "default": "Europe"
  }],
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT region, revenue FROM sales_summary"
        }]
      }
    }
  },
  "dashcards": [{
    "card_id": 47,
    "parameter_mappings": [{
      "parameter_id": "region_param",
      "target": ["dimension", ["field", {"base-type": "type/Text"}, "region"]]
    }],
    "card": {
      "id": 47,
      "name": "Revenue by Region",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
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
}`)

	d, _, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if len(d.Filters) != 1 {
		t.Fatalf("expected dashboard filter to remain visible, got %+v", d.Filters)
	}
	if sql := d.Rows[0].Widgets[0].SQL; strings.Contains(sql, "filters.region") {
		t.Fatalf("expected unscoped Metabase mapping not to apply to widget SQL, got:\n%s", sql)
	}
}

func TestConvertModelBackedLineChartOrdersByBreakout(t *testing.T) {
	input := []byte(`{
  "name": "Model Trend Dashboard",
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT order_date, region, revenue, profit FROM sales_summary"
        }]
      }
    }
  },
  "dashcards": [{
    "card": {
      "id": 50,
      "name": "Revenue Trend",
      "display": "line",
      "query_type": "query",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "breakout": [["field", {"base-type": "type/Date"}, "order_date"]],
          "aggregation": [
            ["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]],
            ["sum", {}, ["field", {"base-type": "type/Decimal"}, "profit"]]
          ]
        }]
      },
      "result_metadata": [
        {"name": "order_date", "base_type": "type/Date", "source": "breakout"},
        {"name": "sum", "base_type": "type/Decimal", "source": "aggregation"},
        {"name": "sum_2", "base_type": "type/Decimal", "source": "aggregation"}
      ],
      "visualization_settings": {
        "graph.dimensions": ["order_date"],
        "graph.metrics": ["sum", "sum_2"]
      }
    }
  }]
}`)

	d, report, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := d.Rows[0].Widgets[0]
	if w.Chart != "line" {
		t.Fatalf("expected line chart, got %+v", w)
	}
	if !strings.Contains(w.SQL, `ORDER BY "order_date" ASC`) {
		t.Fatalf("expected line chart SQL to order by date breakout, got:\n%s", w.SQL)
	}
	if strings.Contains(w.SQL, `ORDER BY "sum" DESC`) {
		t.Fatalf("expected line chart not to order by aggregate, got:\n%s", w.SQL)
	}
	if report.SQLWidgetCount != 1 || report.PlaceholderCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestConvertSemanticImportDoesNotInferUnnamedAggregations(t *testing.T) {
	input := []byte(`{
  "name": "Semantic Safe Import",
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
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
    }
  },
  "dashcards": [{
    "card": {
      "id": 47,
      "name": "Revenue by Region",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
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
}`)

	project, report, err := ConvertProject(input, Options{Semantic: true})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if report.SemanticModelCount != 1 || len(project.SemanticModels) != 1 {
		t.Fatalf("expected one generated semantic model, got report=%+v project=%+v", report, project.SemanticModels)
	}
	if len(project.SemanticModels[0].Model.Metrics) != 0 {
		t.Fatalf("expected no inferred DAC metrics, got %+v", project.SemanticModels[0].Model.Metrics)
	}
	if report.SemanticWidgetCount != 0 || report.SQLWidgetCount != 1 {
		t.Fatalf("expected SQL fallback, got report %+v", report)
	}
	w := project.Dashboard.Rows[0].Widgets[0]
	if w.SQL == "" || !strings.Contains(w.SQL, "WITH source AS") {
		t.Fatalf("expected SQL-backed fallback widget, got %+v", w)
	}
	if w.MetricRef != "" || len(w.MetricRefs) > 0 {
		t.Fatalf("expected no semantic metric refs on fallback widget, got %+v", w)
	}
	if !containsWarning(report.Warnings, "unnamed Metabase aggregations") {
		t.Fatalf("expected warning about unnamed aggregations, got %+v", report.Warnings)
	}

	_, _, err = ConvertProject(input, Options{Semantic: true, SemanticStrict: true})
	if err == nil {
		t.Fatal("expected semantic strict import to fail")
	}
	if !strings.Contains(err.Error(), "unnamed Metabase aggregations") {
		t.Fatalf("expected strict error about unnamed aggregations, got %v", err)
	}
}

func TestConvertSemanticImportUsesExplicitMetabaseMetrics(t *testing.T) {
	input := []byte(`{
  "name": "Semantic Metric Import",
  "parameters": [{
    "id": "region_param",
    "slug": "region",
    "name": "Region",
    "type": "category",
    "default": "Europe",
    "values": ["Europe", "APAC"]
  }, {
    "id": "as_of_param",
    "slug": "as_of_date",
    "name": "As Of Date",
    "type": "date/single",
    "default": "today"
  }],
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT order_date, region, category, status, revenue FROM sales_summary;"
        }]
      },
      "result_metadata": [
        {"name": "order_date", "base_type": "type/Date"},
        {"name": "region", "base_type": "type/Text"},
        {"name": "category", "base_type": "type/Text"},
        {"name": "status", "base_type": "type/Text"},
        {"name": "revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "x-dac-metabase-metrics": {
    "200": {
      "id": 200,
      "name": "Official Revenue",
      "type": "metric",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "filter": ["=", ["field", {"base-type": "type/Text"}, "status"], "paid"],
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      }
    }
  },
  "dashcards": [{
    "row": 0,
    "col": 0,
    "size_x": 12,
    "size_y": 6,
    "parameter_mappings": [{
      "parameter_id": "region_param",
      "card_id": 47,
      "target": ["dimension", ["field", {"base-type": "type/Text"}, "region"]]
    }, {
      "parameter_id": "as_of_param",
      "card_id": 47,
      "target": ["dimension", ["field", {"base-type": "type/Date"}, "order_date"]]
    }],
    "card": {
      "id": 47,
      "name": "Official Revenue by Category",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "breakout": [["field", {"base-type": "type/Text"}, "category"]],
          "aggregation": [["metric", 200]]
        }]
      }
    }
  }]
}`)

	project, report, err := ConvertProject(input, Options{Semantic: true, Connection: "warehouse"})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if report.SemanticModelCount != 1 || report.SemanticWidgetCount != 1 || report.SQLWidgetCount != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}
	model := project.SemanticModels[0].Model
	if model.Name != "sales_summary_model" {
		t.Fatalf("unexpected semantic model name: %q", model.Name)
	}
	if len(model.Metrics) != 1 || model.Metrics[0].Name != "official_revenue" {
		t.Fatalf("expected explicit metric import, got %+v", model.Metrics)
	}
	if model.Metrics[0].Expression != `SUM("revenue")` {
		t.Fatalf("unexpected metric expression: %q", model.Metrics[0].Expression)
	}
	if model.Metrics[0].Filter != `"status" = 'paid'` {
		t.Fatalf("expected explicit metric filter to be imported, got %q", model.Metrics[0].Filter)
	}
	if strings.Contains(model.Source.Table, "sales_summary;\n)") {
		t.Fatalf("expected trailing source SQL terminator to be stripped, got:\n%s", model.Source.Table)
	}
	w := project.Dashboard.Rows[0].Widgets[0]
	if w.SQL != "" {
		t.Fatalf("semantic widget should not have inline SQL, got %q", w.SQL)
	}
	if w.Model != "sales_summary_model" || w.Dimension != "category" {
		t.Fatalf("unexpected semantic widget model/dimension: %+v", w)
	}
	if len(w.MetricRefs) != 1 || w.MetricRefs[0] != "official_revenue" {
		t.Fatalf("unexpected semantic metric refs: %+v", w.MetricRefs)
	}
	if len(w.Filters) != 2 || w.Filters[0].Dimension != "region" || w.Filters[0].Operator != "equals" {
		t.Fatalf("expected semantic dashboard filter mapping, got %+v", w.Filters)
	}
	if w.Filters[1].Dimension != "order_date" || w.Filters[1].Operator != "equals" {
		t.Fatalf("expected semantic single-date filter to use equality, got %+v", w.Filters)
	}
	if err := dashboard.Validate(project.Dashboard); err != nil {
		t.Fatalf("generated semantic dashboard should validate: %v", err)
	}
}

func TestConvertSemanticImportIgnoresUnscopedDashboardFilterMapping(t *testing.T) {
	input := []byte(`{
  "name": "Semantic Metric Import",
  "parameters": [{
    "id": "region_param",
    "slug": "region",
    "name": "Region",
    "type": "category",
    "default": "Europe"
  }],
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT region, category, revenue FROM sales_summary"
        }]
      },
      "result_metadata": [
        {"name": "region", "base_type": "type/Text"},
        {"name": "category", "base_type": "type/Text"},
        {"name": "revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "x-dac-metabase-metrics": {
    "200": {
      "id": 200,
      "name": "Official Revenue",
      "type": "metric",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      }
    }
  },
  "dashcards": [{
    "card_id": 47,
    "parameter_mappings": [{
      "parameter_id": "region_param",
      "target": ["dimension", ["field", {"base-type": "type/Text"}, "region"]]
    }],
    "card": {
      "id": 47,
      "name": "Official Revenue by Category",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "breakout": [["field", {"base-type": "type/Text"}, "category"]],
          "aggregation": [["metric", 200]]
        }]
      }
    }
  }]
}`)

	project, _, err := ConvertProject(input, Options{Semantic: true})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	w := project.Dashboard.Rows[0].Widgets[0]
	if w.Model == "" || len(w.MetricRefs) != 1 {
		t.Fatalf("expected semantic widget conversion, got %+v", w)
	}
	if len(w.Filters) != 0 {
		t.Fatalf("expected unscoped Metabase mapping not to apply to semantic widget, got %+v", w.Filters)
	}
}

func TestConvertSemanticImportIgnoresUnreferencedMetabaseInventory(t *testing.T) {
	input := []byte(`{
  "name": "Semantic Metric Import",
  "models": {
    "99": {
      "id": 99,
      "name": "Unrelated Metabase Model",
      "type": "model"
    }
  },
  "metrics": {
    "100": {
      "id": 100,
      "name": "Unrelated Metabase Metric",
      "type": "metric",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 99,
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      }
    }
  },
  "x-dac-metabase-source-cards": {
    "46": {
      "id": 46,
      "name": "Sales Summary Model",
      "type": "model",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "native": "SELECT category, revenue FROM sales_summary"
        }]
      },
      "result_metadata": [
        {"name": "category", "base_type": "type/Text"},
        {"name": "revenue", "base_type": "type/Decimal"}
      ]
    }
  },
  "x-dac-metabase-metrics": {
    "200": {
      "id": 200,
      "name": "Official Revenue",
      "type": "metric",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "aggregation": [["sum", {}, ["field", {"base-type": "type/Decimal"}, "revenue"]]]
        }]
      }
    }
  },
  "dashcards": [{
    "card": {
      "id": 47,
      "name": "Official Revenue by Category",
      "display": "bar",
      "dataset_query": {
        "database": 2,
        "stages": [{
          "source-card": 46,
          "breakout": [["field", {"base-type": "type/Text"}, "category"]],
          "aggregation": [["metric", 200]]
        }]
      }
    }
  }]
}`)

	_, report, err := ConvertProject(input, Options{Semantic: true})
	if err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if report.SemanticModelCount != 1 || report.SemanticWidgetCount != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if containsWarning(report.Warnings, "Unrelated Metabase") {
		t.Fatalf("expected unreferenced Metabase inventory to be ignored, got warnings %+v", report.Warnings)
	}
}

func TestConvertStrictUnsupportedMBQLFails(t *testing.T) {
	input := []byte(`{
  "name": "Query Builder Dashboard",
  "ordered_cards": [{
    "card": {
      "name": "Built with GUI",
      "display": "bar",
      "dataset_query": {"type": "query", "query": {"source-table": 1}}
    }
  }]
}`)

	_, _, err := Convert(input, Options{Strict: true})
	if err == nil {
		t.Fatal("expected strict conversion to fail")
	}
	if !strings.Contains(err.Error(), "Built with GUI") {
		t.Fatalf("expected card name in error, got %v", err)
	}
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}
