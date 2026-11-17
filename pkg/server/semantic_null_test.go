package server

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"gopkg.in/yaml.v3"
)

func TestSemanticNullSelection(t *testing.T) {
	// Include the literal string "null" alongside a true SQL NULL.
	model := &sem.Model{Name: "orders", Source: sem.Source{Table: "(VALUES ('US'), ('DE'), ('FR'), (NULL), ('null')) AS orders(country)"}, Dimensions: []sem.Dimension{{Name: "country", Type: "string"}}, Metrics: []sem.Metric{{Name: "order_count", Expression: "count(*)"}}}
	cases := []struct {
		operator, value, predicate string
		count                      int
	}{
		{"in", `[US]`, "country IN ('US')", 1},
		{"in", `[US, null]`, "(country IN ('US') OR country IS NULL)", 2},
		{"in", `[US, "null"]`, "country IN ('US', 'null')", 2},
		{"in", `[null]`, "country IS NULL", 1},
		{"not_in", `[US]`, "(country NOT IN ('US') OR country IS NULL)", 4},
		{"not_in", `[US, null]`, "(country NOT IN ('US') AND country IS NOT NULL)", 3},
		{"not_in", `[US, "null"]`, "(country NOT IN ('US', 'null') OR country IS NULL)", 3},
		{"not_in", `[null]`, "country IS NOT NULL", 4},
		{"equals", `US`, "country = 'US'", 1},
		{"not_equals", `US`, "(country != 'US' OR country IS NULL)", 4},
	}
	for _, tc := range cases {
		t.Run(tc.operator+tc.value, func(t *testing.T) {
			var filter dashboard.SemanticQueryFilter
			if err := yaml.Unmarshal([]byte("dimension: country\noperator: "+tc.operator+"\nvalue: "+tc.value), &filter); err != nil {
				t.Fatal(err)
			}
			// Confirm JSON round trips retain YAML's null/string distinction.
			encoded, err := json.Marshal(filter)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &filter); err != nil {
				t.Fatal(err)
			}
			for _, named := range []bool{false, true} {
				d := &dashboard.Dashboard{Name: "Orders", Model: "orders"}
				widget := dashboard.Widget{Name: "Orders", Type: dashboard.WidgetTypeTable, Dimensions: []dashboard.SemanticDimensionRef{{Name: "country"}}, MetricRefs: []string{"order_count"}, Filters: []dashboard.SemanticQueryFilter{filter}}
				if named {
					d.Queries = map[string]dashboard.Query{"orders": {Model: "orders", Dimensions: widget.Dimensions, Metrics: widget.MetricRefs, Filters: widget.Filters}}
					widget = dashboard.Widget{Name: "Orders", Type: dashboard.WidgetTypeTable, QueryRef: "orders"}
				}
				d.SetProjectContext("", map[string]*sem.Model{"orders": model}, nil)
				job, _, err := d.ResolveWidgetSemanticJob(&widget)
				if err != nil {
					t.Fatal(err)
				}
				for _, templated := range []bool{false, true} {
					if templated {
						job.Query.Filters[0].Value = "{{ filters.country }}"
					}
					for _, total := range []bool{false, true} {
						sql, _, _, err := compileSemanticQuery(job, map[string]any{"country": filter.Value}, total)
						if err != nil {
							t.Fatal(err)
						}
						if !strings.Contains(sql, "WHERE "+tc.predicate) {
							t.Fatalf("missing %s: %s", tc.predicate, sql)
						}
						if binary, err := exec.LookPath("duckdb"); err == nil {
							query := "SELECT count(*) FROM (" + sql + ") result"
							if total {
								query = "SELECT order_count FROM (" + sql + ") result"
							}
							out, err := exec.Command(binary, "-csv", "-noheader", "-c", query).CombinedOutput()
							if err != nil {
								t.Fatalf("%v: %s\n%s", err, out, sql)
							}
							if strings.TrimSpace(string(out)) != strconv.Itoa(tc.count) {
								t.Fatalf("want %d rows, got %s\n%s", tc.count, out, sql)
							}
						}
					}
				}
			}
		})
	}
}

func TestSemanticNullFilterKeepsLiteralsOpaque(t *testing.T) {
	model := &sem.Model{Name: "orders", Source: sem.Source{Table: "orders"}, Dimensions: []sem.Dimension{{Name: "country", Type: "string"}}}
	for _, dialect := range []string{"duckdb", "bigquery"} {
		job := &dashboard.SemanticJob{Model: model, Query: sem.Query{Dimensions: []sem.DimensionRef{{Name: "country"}}, Filters: []sem.Filter{{Dimension: "country", Operator: "in", Value: "{{ filters.country }}"}}}}
		sql, _, _, err := compileSemanticJob(job, map[string]any{"country": []any{"Kids' {missing} Toys", nil}}, dialect)
		if err != nil {
			t.Fatal(err)
		}
		want := "'Kids'' {missing} Toys'"
		if dialect == "bigquery" {
			want = `"Kids' {missing} Toys"`
		}
		if !strings.Contains(sql, want) || !strings.Contains(sql, "OR country IS NULL") {
			t.Fatal(sql)
		}
	}
}

func TestSemanticNullThroughDashboardFilters(t *testing.T) {
	model := &sem.Model{Name: "orders", Source: sem.Source{Table: "orders"}, Dimensions: []sem.Dimension{{Name: "country", Type: "string"}, {Name: "inactive", Type: "boolean", Expression: "NOT active"}}, Metrics: []sem.Metric{{Name: "order_count", Expression: "count(*)"}}}
	d := &dashboard.Dashboard{Name: "Orders", Model: "orders", Filters: []dashboard.Filter{{Name: "country", Type: "select", Multiple: true, Default: []any{"US", nil}}}}
	d.Rows = []dashboard.Row{{Widgets: []dashboard.Widget{
		{Name: "Templated", Type: dashboard.WidgetTypeTable, Dimensions: []dashboard.SemanticDimensionRef{{Name: "country"}}, MetricRefs: []string{"order_count"}, Filters: []dashboard.SemanticQueryFilter{{Dimension: "country", Operator: "in", Value: "{{ filters.country }}"}}},
		{Name: "Expression", Type: dashboard.WidgetTypeTable, Dimensions: []dashboard.SemanticDimensionRef{{Name: "inactive"}}, MetricRefs: []string{"order_count"}, Filters: []dashboard.SemanticQueryFilter{{Dimension: "inactive", Operator: "in", Value: []any{nil}}}},
	}}}
	d.SetProjectContext("", map[string]*sem.Model{"orders": model}, nil)
	// A multi-select default with a null member must pass filter validation.
	jobs, err := ResolveWidgetJobs(d, d.DefaultFilters())
	if err != nil {
		t.Fatal(err)
	}
	// Expression dimensions are grouped so IS NULL applies to the whole expression.
	for i, want := range []string{"WHERE (country IN ('US') OR country IS NULL)", "WHERE (NOT active) IS NULL"} {
		if !strings.Contains(jobs[i].SQL, want) {
			t.Fatalf("missing %s: %s", want, jobs[i].SQL)
		}
	}
}
