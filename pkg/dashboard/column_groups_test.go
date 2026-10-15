package dashboard

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestColumnGroupsDefinitionRoundTrip(t *testing.T) {
	definition := []byte(`name: Financials
rows:
  - widgets:
      - name: Results
        type: table
        data:
          columns: [revenue, cost, profit]
          rows: [[100, 60, 40]]
        columns:
          - {name: profit, number: currency, group: financials, collapsed: true, frozen: true}
          - {name: revenue, number: currency, group: financials}
          - {name: cost, number: currency, group: financials}
`)
	d, err := LoadDefinition(definition, nil)
	assertNoErr(t, err)
	assertNoErr(t, Validate(d))
	want := []TableColumn{
		{Name: "profit", Number: "currency", Group: "financials", Collapsed: boolPtr(true), Frozen: boolPtr(true)},
		{Name: "revenue", Number: "currency", Group: "financials"},
		{Name: "cost", Number: "currency", Group: "financials"},
	}
	if !reflect.DeepEqual(d.Rows[0].Widgets[0].Columns, want) {
		t.Fatalf("group metadata or column order lost during YAML loading: %+v", d.Rows[0].Widgets[0].Columns)
	}
	encoded, err := json.Marshal(d)
	assertNoErr(t, err)
	var roundTrip Dashboard
	assertNoErr(t, json.Unmarshal(encoded, &roundTrip))
	if !reflect.DeepEqual(roundTrip.Rows[0].Widgets[0].Columns, want) {
		t.Fatalf("group metadata lost in API JSON: %s", encoded)
	}
}

func TestColumnGroupsTSXAndTabs(t *testing.T) {
	source := `export default <Dashboard name="Finance"><Row>
      <WidgetTabs name="Results"><Table name="Details" sql="SELECT revenue, cost, profit, orders, average FROM financials"
        columns={[
          {name: "profit", group: "financials", collapsed: false, frozen: false},
          {name: "revenue", group: "financials"},
          {name: "cost", group: "orders"},
          {name: "orders", group: "financials", collapsed: true, frozen: true},
          {name: "average", group: "financials"},
        ]} />
      </WidgetTabs></Row></Dashboard>`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)
	assertNoErr(t, Validate(d))
	columns := d.Rows[0].Widgets[0].Tabs[0].Columns
	if len(columns) != 5 || columns[0].Group != "financials" || columns[0].Collapsed == nil || *columns[0].Collapsed || columns[1].Collapsed != nil || columns[1].Frozen != nil || columns[0].Frozen == nil || *columns[0].Frozen || columns[3].Group != "financials" || columns[3].Collapsed == nil || !*columns[3].Collapsed || columns[3].Frozen == nil || !*columns[3].Frozen {
		t.Fatalf("group metadata lost during TSX tab loading: %+v", columns)
	}
}

func TestColumnGroupsTSXUndefinedProps(t *testing.T) {
	source := `const cols = ["profit", "revenue", "cost"];
export default <Dashboard name="Finance"><Row>
      <Table name="Details" sql="SELECT profit, revenue, cost, region FROM financials"
        columns={[
          ...cols.map((name, i) => ({name, group: "financials", collapsed: i === 0 ? true : undefined, frozen: undefined})),
          {name: "region", collapsed: undefined},
        ]} />
      </Row></Dashboard>`
	d, err := evalTSX(source, "test.tsx", &tsxConfig{})
	assertNoErr(t, err)
	assertNoErr(t, Validate(d))
	columns := d.Rows[0].Widgets[0].Columns
	if columns[0].Collapsed == nil || !*columns[0].Collapsed || columns[1].Collapsed != nil || columns[1].Frozen != nil || columns[3].Collapsed != nil {
		t.Fatalf("undefined TSX props should be treated as unset: %+v", columns)
	}
}

func TestColumnGroupsValidation(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*Widget)
		want   string
	}{
		{name: "initially collapsed"},
		{name: "initially expanded", modify: func(w *Widget) { w.Columns[0].Collapsed = boolPtr(false) }},
		{name: "default expanded", modify: func(w *Widget) { w.Columns[0].Collapsed = nil }},
		{name: "initially frozen", modify: func(w *Widget) { w.Columns[0].Frozen = boolPtr(true) }},
		{name: "initially unfrozen", modify: func(w *Widget) { w.Columns[0].Frozen = boolPtr(false) }},
		{name: "default unfrozen", modify: func(w *Widget) { w.Columns[0].Frozen = nil }},
		{name: "true frozen on later member", modify: func(w *Widget) { w.Columns[1].Frozen = boolPtr(true) }, want: "frozen: only allowed on the first column"},
		{name: "false frozen on later member", modify: func(w *Widget) { w.Columns[1].Frozen = boolPtr(false) }, want: "frozen: only allowed on the first column"},
		{name: "true on later member", modify: func(w *Widget) { w.Columns[1].Collapsed = boolPtr(true) }, want: "only allowed on the first column"},
		{name: "false on later member", modify: func(w *Widget) { w.Columns[1].Collapsed = boolPtr(false) }, want: "only allowed on the first column"},
		{name: "true without group", modify: func(w *Widget) { w.Columns[0].Group = "" }, want: "collapsed: requires group"},
		{name: "false without group", modify: func(w *Widget) { w.Columns[0].Group = ""; w.Columns[0].Collapsed = boolPtr(false) }, want: "collapsed: requires group"},
		{name: "blank group", modify: func(w *Widget) { w.Columns[0].Group = "  " }, want: "group: must not be blank"},
		{name: "singleton among other groups", modify: func(w *Widget) { w.Columns[0].Group = "other" }},
		{name: "singleton without collapsed", modify: func(w *Widget) {
			w.Columns = []TableColumn{{Name: "orders", Group: "orders"}, {Name: "average_order"}}
		}},
		{name: "singleton with collapsed", modify: func(w *Widget) {
			w.Columns = []TableColumn{{Name: "orders", Group: "orders", Collapsed: boolPtr(true)}}
		}},
		{name: "singleton with frozen", modify: func(w *Widget) {
			w.Columns = []TableColumn{{Name: "orders", Group: "orders", Frozen: boolPtr(true)}}
		}},
		{name: "hidden summary", modify: func(w *Widget) { w.Columns[0].Hidden = true }, want: "must not be hidden"},
		{name: "hidden detail", modify: func(w *Widget) { w.Columns[1].Hidden = true }, want: "must not be hidden"},
		{name: "non-table", modify: func(w *Widget) { w.Type = WidgetTypeText; w.Content = "hello" }, want: "only supported on plain tables"},
		{name: "pivot", modify: func(w *Widget) {
			w.Type = WidgetTypePivotTable
			w.Pivot = &PivotConfig{Rows: []PivotField{{Field: "revenue"}}, Values: []PivotValue{{Field: "profit"}}}
		}, want: "only supported on plain tables"},
		{name: "separated names create independent blocks", modify: func(w *Widget) {
			w.Columns = []TableColumn{
				{Name: "profit", Group: "financials", Collapsed: boolPtr(true), Frozen: boolPtr(true)},
				{Name: "revenue", Group: "financials"},
				{Name: "orders", Group: "orders", Collapsed: boolPtr(false)},
				{Name: "average", Group: "orders"},
				{Name: "forecast", Group: "financials", Collapsed: boolPtr(false), Frozen: boolPtr(false)},
				{Name: "target", Group: "financials"},
			}
		}},
		{name: "ungrouped column ends a block", modify: func(w *Widget) {
			w.Columns = []TableColumn{
				{Name: "profit", Group: "financials", Collapsed: boolPtr(true), Frozen: boolPtr(true)},
				{Name: "revenue", Group: "financials"},
				{Name: "region"},
				{Name: "forecast", Group: "financials", Collapsed: boolPtr(true)},
				{Name: "target", Group: "financials"},
			}
		}},
		{name: "singleton block before repeated name", modify: func(w *Widget) {
			w.Columns = []TableColumn{
				{Name: "profit", Group: "financials", Collapsed: boolPtr(true), Frozen: boolPtr(true)},
				{Name: "orders", Group: "orders"},
				{Name: "forecast", Group: "financials", Collapsed: boolPtr(true)},
				{Name: "target", Group: "financials"},
			}
		}},
		{name: "collapsed on later member of second block", modify: func(w *Widget) {
			w.Columns = []TableColumn{
				{Name: "profit", Group: "financials"},
				{Name: "orders", Group: "orders"},
				{Name: "forecast", Group: "financials", Collapsed: boolPtr(true)},
				{Name: "target", Group: "financials", Collapsed: boolPtr(false)},
			}
		}, want: "only allowed on the first column"},
		{name: "frozen on later member of second block", modify: func(w *Widget) {
			w.Columns = []TableColumn{
				{Name: "profit", Group: "financials", Frozen: boolPtr(true)},
				{Name: "orders", Group: "orders"},
				{Name: "forecast", Group: "financials", Frozen: boolPtr(false)},
				{Name: "target", Group: "financials", Frozen: boolPtr(false)},
			}
		}, want: "frozen: only allowed on the first column"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := Widget{Name: "Results", Type: WidgetTypeTable, SQL: "SELECT revenue, cost, profit FROM financials", Columns: []TableColumn{
				{Name: "profit", Group: "financials", Collapsed: boolPtr(true), Frozen: boolPtr(true)},
				{Name: "revenue", Group: "financials"},
				{Name: "cost", Group: "financials"},
			}}
			if tc.modify != nil {
				tc.modify(&w)
			}
			d := &Dashboard{Name: "Financials", Rows: []Row{{Widgets: []Widget{w}}}}
			err := Validate(d)
			if tc.want == "" {
				assertNoErr(t, err)
			} else {
				assertValidationContains(t, err, tc.want)
			}
			// Explicit false must survive API serialization so invalid placement
			// cannot become valid by silently dropping the field.
			encoded, marshalErr := json.Marshal(d)
			assertNoErr(t, marshalErr)
			var decoded Dashboard
			assertNoErr(t, json.Unmarshal(encoded, &decoded))
			decodedErr := Validate(&decoded)
			if tc.want == "" {
				assertNoErr(t, decodedErr)
			} else {
				assertValidationContains(t, decodedErr, tc.want)
			}
		})
	}
}
