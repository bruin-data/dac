package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
)

func TestResolveWidgetQuery_Tabs(t *testing.T) {
	dir := t.TempDir()
	yml := `name: T
connection: c
rows:
  - widgets:
      - name: Sales
        type: tabs
        tabs:
          - name: Revenue
            type: table
            sql: SELECT 1 AS revenue
          - name: Orders
            type: table
            sql: SELECT 2 AS orders
`
	if err := os.WriteFile(filepath.Join(dir, "t.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"Orders", "Sales / Orders"} {
		sql, _, err := resolveWidgetQuery(dir, "T", name)
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if !strings.Contains(sql, "SELECT 2") {
			t.Fatalf("%q resolved to %q, want the Orders tab", name, sql)
		}
	}

	_, _, err := resolveWidgetQuery(dir, "T", "Sales")
	if err == nil || !strings.Contains(err.Error(), "pass one of its tab names: Revenue, Orders") {
		t.Fatalf("expected a tab-name hint, got %v", err)
	}
}

func TestMatchWidget_AmbiguousAndIDs(t *testing.T) {
	d := &dashboard.Dashboard{Name: "T", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{
		{Name: "Sales", Type: dashboard.WidgetTypeTabs, Tabs: []dashboard.Widget{{Name: "Revenue", Type: "table"}}},
		{Type: dashboard.WidgetTypeTabs, Tabs: []dashboard.Widget{{Name: "Revenue", Type: "table"}}}, // title-less
	}}}}

	// "Revenue" is the title-less widget's full label, so it wins over the
	// "Sales / Revenue" tab-name shortcut.
	if id, err := matchWidget(d, "Revenue"); err != nil || id != "r0-w1::Revenue" {
		t.Fatalf("full label: got %q, %v", id, err)
	}
	// Two titled widgets sharing a tab name: the shortcut is ambiguous.
	d2 := &dashboard.Dashboard{Name: "T", Rows: []dashboard.Row{{Widgets: []dashboard.Widget{
		{Name: "Sales", Type: dashboard.WidgetTypeTabs, Tabs: []dashboard.Widget{{Name: "Revenue", Type: "table"}}},
		{Name: "Marketing", Type: dashboard.WidgetTypeTabs, Tabs: []dashboard.Widget{{Name: "Revenue", Type: "table"}}},
	}}}}
	if _, err := matchWidget(d2, "Revenue"); err == nil || !strings.Contains(err.Error(), `matches 2 widgets/tabs`) ||
		!strings.Contains(err.Error(), "(r0-w1::Revenue)") {
		t.Fatalf("expected an ambiguity error listing ids, got %v", err)
	}
	if id, err := matchWidget(d, "Sales / Revenue"); err != nil || id != "r0-w0::Revenue" {
		t.Fatalf("Widget / Tab: got %q, %v", id, err)
	}
	// The title-less widget's tab is reachable by its id.
	if id, err := matchWidget(d, "r0-w1::Revenue"); err != nil || id != "r0-w1::Revenue" {
		t.Fatalf("job id: got %q, %v", id, err)
	}
}
