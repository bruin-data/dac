package dashboard

import "testing"

// ResolvedTab returns the tab itself: nothing is inherited from the container.
func TestResolvedTab_NoInheritance(t *testing.T) {
	parent := Widget{
		Type: WidgetTypeTabs,
		Col:  6,
		Tabs: []Widget{{Name: "A", Type: WidgetTypeChart, Chart: "bar"}, {Name: "B", Type: WidgetTypeTable}},
	}
	a := parent.ResolvedTab(0)
	if a.Type != WidgetTypeChart || a.Chart != "bar" || a.Col != 0 {
		t.Fatalf("tab A should be itself only, got type=%q chart=%q col=%d", a.Type, a.Chart, a.Col)
	}
	if b := parent.ResolvedTab(1); b.Type != WidgetTypeTable || b.Chart != "" {
		t.Fatalf("tab B should not inherit, got type=%q chart=%q", b.Type, b.Chart)
	}
}

// A semantic chart tab gets its x/y derived just like a top-level widget.
func TestPostProcess_SemanticTabEncoding(t *testing.T) {
	d := &Dashboard{
		Model: "sales",
		Rows: []Row{{Widgets: []Widget{{
			Type: WidgetTypeTabs,
			Tabs: []Widget{{Name: "Revenue", Type: WidgetTypeChart, Chart: "bar", Dimension: "month", MetricRefs: []string{"revenue"}}},
		}}}},
	}
	postProcessDashboard(d)
	tab := d.Rows[0].Widgets[0].Tabs[0]
	if tab.XField() != "month" || len(tab.YFields()) != 1 || tab.YFields()[0] != "revenue" {
		t.Fatalf("tab encoding not derived: x=%q y=%v", tab.XField(), tab.YFields())
	}
}

// A widget's named semantic query still sets x/y when the widget also lists dimension/metrics.
func TestPostProcess_NamedQueryEncodingWins(t *testing.T) {
	d := &Dashboard{
		Model:   "sales",
		Queries: map[string]Query{"q": {Model: "sales", Dimensions: []SemanticDimensionRef{{Name: "region"}}, Metrics: []string{"orders"}}},
		Rows: []Row{{Widgets: []Widget{{
			Type: WidgetTypeChart, Chart: "bar", QueryRef: "q", Dimension: "month", MetricRefs: []string{"revenue"},
		}}}},
	}
	postProcessDashboard(d)
	w := d.Rows[0].Widgets[0]
	if w.XField() != "region" || len(w.YFields()) != 1 || w.YFields()[0] != "orders" {
		t.Fatalf("want query encoding x=region y=[orders], got x=%q y=%v", w.XField(), w.YFields())
	}
}
