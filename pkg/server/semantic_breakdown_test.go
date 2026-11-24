package server

import (
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	"gopkg.in/yaml.v3"
)

// A breakdown filter swaps the selected option's dimensions into the widget's
// dimensions and sort, next to fixed dimensions, keeping the real column names.
func TestSemanticBreakdownFilter(t *testing.T) {
	var d dashboard.Dashboard
	if err := yaml.Unmarshal([]byte(`
name: Breakdown
connection: warehouse
model: sales
filters:
  - name: geo
    type: breakdown
    default: Country
    options:
      - label: Country
        dimensions: [country]
      - label: Channel & Country
        dimensions: [channel, country]
      - label: Monthly
        dimensions: [created_at:month]
rows:
  - widgets:
      - name: Revenue
        type: table
        dimensions:
          - name: "{{ filters.geo }}"
          - name: country
        metrics: [revenue]
        sort:
          - name: "{{ filters.geo }}"
`), &d); err != nil {
		t.Fatal(err)
	}
	d.SetProjectContext("", map[string]*sem.Model{"sales": {
		Name:   "sales",
		Source: sem.Source{Table: "orders"},
		Dimensions: []sem.Dimension{
			{Name: "country", Type: "string"},
			{Name: "channel", Type: "string"},
			{Name: "created_at", Type: "time", Granularities: map[string]string{"month": "date_trunc('month', created_at)"}},
		},
		Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}},
	}}, nil)
	if err := dashboard.Validate(&d); err != nil {
		t.Fatal(err)
	}

	job, _, err := d.ResolveWidgetSemanticJob(&d.Rows[0].Widgets[0])
	if err != nil {
		t.Fatal(err)
	}
	sql, _, _, err := compileSemanticJob(job, map[string]any{"geo": "Channel & Country"})
	if err != nil {
		t.Fatal(err)
	}
	// country appears in both the option and the fixed list, so it groups once.
	for _, want := range []string{"SELECT channel AS channel, country AS country, sum(amount)", "GROUP BY 1, 2 ", "ORDER BY channel ASC, country ASC"} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q:\n%s", want, sql)
		}
	}
	sql, _, _, err = compileSemanticJob(job, map[string]any{"geo": "Monthly"})
	if err != nil || !strings.Contains(sql, "date_trunc('month', created_at) AS created_at, country AS country") {
		t.Errorf("monthly option: err=%v\n%s", err, sql)
	}

	if err := d.ValidateFilterValues(map[string]any{"geo": "region"}); err == nil {
		t.Error("expected a value outside the option labels to be rejected")
	}
}
