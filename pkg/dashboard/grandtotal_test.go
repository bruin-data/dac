package dashboard

import (
	"encoding/json"
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

func grandTotalModel() *sem.Model {
	return &sem.Model{Name: "sales", Source: sem.Source{Table: "orders"}, Dimensions: []sem.Dimension{{Name: "region", Type: "string"}}, Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}}}
}
func setGrandTotalModel(d *Dashboard) {
	d.SetProjectContext("", map[string]*sem.Model{"sales": grandTotalModel()}, nil)
}

func TestGrandTotalDefinitionRoundTrip(t *testing.T) {
	d, err := LoadDefinition([]byte(`name: GrandTotal
model: sales
rows:
 - widgets:
    - name: Sales
      type: table
      dimensions: [{name: region}]
      metrics: [revenue]
      grandTotal: {label: Overall}
`), nil)
	assertNoErr(t, err)
	setGrandTotalModel(d)
	assertNoErr(t, Validate(d))
	encoded, err := json.Marshal(d)
	assertNoErr(t, err)
	var back Dashboard
	assertNoErr(t, json.Unmarshal(encoded, &back))
	setGrandTotalModel(&back)
	assertNoErr(t, Validate(&back))
	if back.Rows[0].Widgets[0].GrandTotal.(map[string]interface{})["label"] != "Overall" {
		t.Fatal("total setting lost in JSON")
	}
}

func TestGrandTotalTSXAndTabs(t *testing.T) {
	d, err := evalTSX(`export default <Dashboard name="Finance" model="sales"><Row><WidgetTabs name="Results"><Table name="Sales" dimensions={[{name:"region"}]} metrics={["revenue"]} grandTotal={{label:"Overall"}} /></WidgetTabs></Row></Dashboard>`, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)
	setGrandTotalModel(d)
	assertNoErr(t, Validate(d))
	if d.Rows[0].Widgets[0].Tabs[0].GrandTotal == nil {
		t.Fatal("total setting lost in TSX")
	}
}

func TestGrandTotalValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    Widget
		want string
	}{
		{"SQL", Widget{Type: WidgetTypeTable, SQL: "select 1", GrandTotal: true}, "semantic-model"},
		{"inline", Widget{Type: WidgetTypeTable, Data: &WidgetData{Columns: []string{"x"}, Rows: [][]any{{1}}}, GrandTotal: true}, "semantic-model"},
		{"pivot", Widget{Type: WidgetTypePivotTable, GrandTotal: true}, "plain tables"},
		{"no metrics", Widget{Type: WidgetTypeTable, Model: "sales", Dimensions: []SemanticDimensionRef{{Name: "region"}}, GrandTotal: true}, "at least one"},
		{"bad option", Widget{Type: WidgetTypeTable, GrandTotal: map[string]interface{}{"frozen": true}}, "unknown grandTotal"},
		{"bad type", Widget{Type: WidgetTypeTable, GrandTotal: "yes"}, "boolean"},
		{"bad label", Widget{Type: WidgetTypeTable, GrandTotal: map[string]interface{}{"label": 1}}, "string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &Dashboard{}
			setGrandTotalModel(d)
			if err := tc.w.ValidateGrandTotal(d); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	w := Widget{Type: WidgetTypeTable, SQL: "select 1", GrandTotal: false}
	assertNoErr(t, w.ValidateGrandTotal(&Dashboard{}))
}

func TestGrandTotalAllowsWindowMetricDependencies(t *testing.T) {
	model := grandTotalModel()
	model.Metrics = append(model.Metrics, sem.Metric{Name: "running", Expression: "{revenue}", Window: &sem.Window{Type: "running_total", OrderBy: "region"}}, sem.Metric{Name: "derived", Expression: "{running} * 2"})
	for _, name := range []string{"running", "derived"} {
		d := &Dashboard{Model: "sales"}
		d.SetProjectContext("", map[string]*sem.Model{"sales": model}, nil)
		w := Widget{Type: WidgetTypeTable, GrandTotal: true, MetricRefs: []string{name}}
		if err := w.ValidateGrandTotal(d); err != nil {
			t.Fatalf("window metric %q should allow a blank grand total: %v", name, err)
		}
	}
}
