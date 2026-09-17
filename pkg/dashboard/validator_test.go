package dashboard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Validator tests
// ---------------------------------------------------------------------------

func TestValidate_ValidDashboard(t *testing.T) {
	d, err := LoadFile("../../testdata/dashboards/sales.yml")
	assertNoErr(t, err)

	err = Validate(d)
	assertNoErr(t, err)
}

// ---------------------------------------------------------------------------
// Widget-internal tabs
// ---------------------------------------------------------------------------

func tabbedChartWidget() Widget {
	return Widget{
		Name:  "Sales",
		Type:  WidgetTypeChart,
		Chart: "bar",
		Tabs: []Widget{
			{
				Name: "Revenue",
				Data: &WidgetData{Columns: []string{"month", "revenue"}, Rows: [][]any{{"Jan", 1}}},
				X:    &AxisEncoding{Field: "month"},
				Y:    &AxisEncoding{Field: "revenue"},
			},
			{
				Name: "Orders",
				Data: &WidgetData{Columns: []string{"month", "orders"}, Rows: [][]any{{"Jan", 1}}},
				X:    &AxisEncoding{Field: "month"},
				Y:    &AxisEncoding{Field: "orders"},
			},
		},
	}
}

func dashboardWith(w Widget) *Dashboard {
	return &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{w}}}}
}

func TestValidate_WidgetTabs_Valid(t *testing.T) {
	assertNoErr(t, Validate(dashboardWith(tabbedChartWidget())))
}

func TestValidate_WidgetTabs_TabInheritsTypeError(t *testing.T) {
	// A type-less chart tab missing y must fail — proves tabs validate by type.
	w := tabbedChartWidget()
	w.Tabs[1].Y = nil
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, `tab 2 ("Orders")`)
}

func TestValidate_WidgetTabs_ContainerHasDataSource(t *testing.T) {
	w := tabbedChartWidget()
	w.SQL = "SELECT 1"
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, "a widget with tabs must not set its own sql")
}

func TestValidate_WidgetTabs_TabMissingName(t *testing.T) {
	w := tabbedChartWidget()
	w.Tabs[0].Name = ""
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, "tab 1: name is required")
}

func TestValidate_WidgetTabs_NestedRejected(t *testing.T) {
	w := tabbedChartWidget()
	w.Tabs[0].Tabs = []Widget{{Name: "inner"}}
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, "tabs cannot be nested")
}

func TestValidate_WidgetTabs_ImageInheritsSrc(t *testing.T) {
	// src on the container must be inherited by each data-only tab.
	w := Widget{
		Name: "Gallery",
		Type: WidgetTypeImage,
		Src:  "photo",
		Tabs: []Widget{
			{Name: "A", Data: &WidgetData{Columns: []string{"photo"}, Rows: [][]any{{"u"}}}},
			{Name: "B", Data: &WidgetData{Columns: []string{"photo"}, Rows: [][]any{{"u"}}}},
		},
	}
	assertNoErr(t, Validate(dashboardWith(w)))
}

func TestValidate_WidgetTabs_DuplicateName(t *testing.T) {
	w := tabbedChartWidget()
	w.Tabs[1].Name = w.Tabs[0].Name
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, "duplicate tab name")
}

func TestValidate_WidgetTabs_TypeMustMatch(t *testing.T) {
	w := tabbedChartWidget()
	w.Tabs[1].Type = WidgetTypeTable
	err := Validate(dashboardWith(w))
	assertErr(t, err)
	assertValidationContains(t, err, "must match the widget's type")
}

func TestValidate_MissingName(t *testing.T) {
	d := &Dashboard{
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "name is required")
}

func TestValidate_MissingRows(t *testing.T) {
	d := &Dashboard{Name: "test"}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "at least one row is required")
}

func TestValidate_EmptyRow(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "at least one widget is required")
}

func TestValidate_WidgetMissingName(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "name is required")
}

func TestValidate_WidgetMissingType(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "type is required")
}

func TestValidate_ColumnSumExceeds12(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{
				{Name: "a", Type: WidgetTypeText, Content: "hi", Col: 8},
				{Name: "b", Type: WidgetTypeText, Content: "hi", Col: 6},
			}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "exceeds 12")
}

func TestValidate_TextWidgetMissingContent(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "content is required")
}

func TestValidate_ChartWidgetMissingChartType(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeChart, SQL: "SELECT 1"}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "chart type is required")
}

func TestValidate_VegaLiteChart(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "Layered", Type: WidgetTypeChart, Chart: "vega-lite", SQL: "SELECT 1 AS value",
			Spec: map[string]any{
				"data": map[string]any{"name": "dac"},
				"layer": []any{
					map[string]any{"mark": "line"},
					map[string]any{"mark": "point"},
				},
			},
		}}}},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_VegaLiteChartRequiresSpec(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "Layered", Type: WidgetTypeChart, Chart: "vega-lite", SQL: "SELECT 1 AS value",
		}}}},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "spec is required for vega-lite charts")
}

func TestValidate_VegaLiteChartRejectsRemoteData(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "Remote", Type: WidgetTypeChart, Chart: "vega-lite", SQL: "SELECT 1 AS value",
			Spec: map[string]any{
				"mark": "line",
				"data": map[string]any{"url": "https://example.com/data.json"},
			},
		}}}},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "spec.data must be { name: dac }")
	assertValidationContains(t, err, "data.url is not allowed")
}

func TestValidate_VegaLiteSpecOnlyOnVegaLiteCharts(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "Line", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1 AS x, 2 AS y",
			X: &AxisEncoding{Field: "x"}, Y: &AxisEncoding{Field: "y"},
			Spec: map[string]any{"mark": "line"},
		}}}},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "spec is only valid on vega-lite charts")
}

func TestValidate_StackedRequiresColor(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "bar", SQL: "SELECT 1",
				X: &AxisEncoding{Field: "month"}, Y: &AxisEncoding{Field: "revenue"},
				Stacked: boolPtr(true),
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "stacked requires color")
}

func TestValidate_StackedOnlyOnBarCharts(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "area", SQL: "SELECT 1",
				X: &AxisEncoding{Field: "month"}, Y: &AxisEncoding{Field: "revenue"},
				Stacked: boolPtr(true), Color: &ColorEncoding{Field: "region"},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "stacked is only valid on bar charts")
}

func TestValidate_StackedBarWithColor(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "bar", SQL: "SELECT 1",
				X: &AxisEncoding{Field: "month"}, Y: &AxisEncoding{Field: "revenue"},
				Stacked: boolPtr(true), Color: &ColorEncoding{Field: "region"},
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_ShowValuesOnlyOnHeatmap(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "bar", SQL: "SELECT 1",
				X: &AxisEncoding{Field: "month"}, Y: &AxisEncoding{Field: "revenue"},
				ShowValues: boolPtr(true),
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "showValues is only valid on heatmap charts")
}

func TestValidate_ShowValuesOnHeatmap(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "heatmap", SQL: "SELECT 1",
				X: &AxisEncoding{Field: "day"}, Y: &AxisEncoding{Field: "region"},
				Value: &ValueEncoding{Field: "orders"}, ShowValues: boolPtr(true),
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_ColorScale(t *testing.T) {
	heatmap := func(cs *ColorScale, chart string) *Dashboard {
		return &Dashboard{
			Name: "test",
			Rows: []Row{
				{Widgets: []Widget{{
					Name: "w", Type: WidgetTypeChart, Chart: chart, SQL: "SELECT 1",
					X: &AxisEncoding{Field: "day"}, Y: &AxisEncoding{Field: "region"},
					Value: &ValueEncoding{Field: "orders"}, ColorScale: cs,
				}}},
			},
		}
	}

	cases := []struct {
		name  string
		chart string
		scale *ColorScale
		want  string // "" means valid
	}{
		{"auto min/max", "heatmap", &ColorScale{BackgroundColor: []string{"red", "white", "green"}}, ""},
		{"pinned anchors", "heatmap", &ColorScale{BackgroundColor: []string{"red", "white", "green"}, Range: []float64{-500, 0, 500}, Unit: "absolute"}, ""},
		{"percentile", "heatmap", &ColorScale{BackgroundColor: []string{"white", "indigo"}, Range: []float64{10, 90}, Unit: "percentile"}, ""},
		{"one color", "heatmap", &ColorScale{BackgroundColor: []string{"red"}}, "at least 2 colors"},
		{"anchor count mismatch", "heatmap", &ColorScale{BackgroundColor: []string{"red", "green"}, Range: []float64{0, 1, 2}}, "one anchor per color"},
		{"bad unit", "heatmap", &ColorScale{BackgroundColor: []string{"red", "green"}, Unit: "quantile"}, "must be absolute, percent or percentile"},
		{"wrong chart", "bar", &ColorScale{BackgroundColor: []string{"red", "green"}}, "only valid on heatmap charts"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(heatmap(tc.scale, tc.chart))
			if tc.want == "" {
				assertNoErr(t, err)
				return
			}
			assertErr(t, err)
			assertValidationContains(t, err, tc.want)
		})
	}
}

func TestValidate_DualAxisValid(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "combo", SQL: "SELECT 1",
				Lines: []string{"conversion_rate"},
				X:     &AxisEncoding{Field: "month"},
				Y:     &AxisEncoding{Field: []string{"revenue"}},
				Y2:    &AxisEncoding{Field: []string{"conversion_rate"}, Format: ".1%"},
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_DualAxisUnsupportedChart(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "pie", SQL: "SELECT 1",
				Label: "region", Value: &ValueEncoding{Field: "revenue"},
				Y2: &AxisEncoding{Field: []string{"conversion_rate"}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "y2 is only supported on line, area, bar, and combo charts")
}

func TestValidate_DualAxisColumnOverlap(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1",
				X:  &AxisEncoding{Field: "month"},
				Y:  &AxisEncoding{Field: []string{"revenue", "orders"}},
				Y2: &AxisEncoding{Field: []string{"orders"}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "also appears on the left y axis")
}

func TestValidate_DualAxisRequiresField(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1",
				X:  &AxisEncoding{Field: "month"},
				Y:  &AxisEncoding{Field: []string{"revenue"}},
				Y2: &AxisEncoding{Title: "Orders"}, // no field
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "y2.field is required")
}

func TestValidate_DualAxisTypeMustBeNumber(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1",
				X:  &AxisEncoding{Field: "month"},
				Y:  &AxisEncoding{Field: []string{"revenue"}},
				Y2: &AxisEncoding{Field: []string{"orders"}, Type: "category"},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "y2.type must be number")
}

func TestValidate_DualAxisRejectsHorizontalBar(t *testing.T) {
	horizontal := true
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "bar", SQL: "SELECT 1",
				X:          &AxisEncoding{Field: "month"},
				Y:          &AxisEncoding{Field: []string{"revenue"}},
				Y2:         &AxisEncoding{Field: []string{"orders"}},
				Horizontal: &horizontal,
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "y2 cannot be combined with horizontal bars")
}

func TestValidate_DualAxisRejectsColor(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1",
				X:     &AxisEncoding{Field: "month"},
				Y:     &AxisEncoding{Field: []string{"revenue"}},
				Y2:    &AxisEncoding{Field: []string{"orders"}},
				Color: &ColorEncoding{Field: "region"},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "y2 cannot be combined with color")
}

func TestValidate_PivotValid(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, product, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:    []PivotField{{Field: "region", Order: "asc", ShowTotals: true}},
					Columns: []PivotField{{Field: "product"}},
					Values:  []PivotValue{{Field: "sales", Summarize: "sum", Label: "Total Sales", Format: []FormatLayer{{BackgroundColor: []interface{}{"#FECACA", "#BBF7D0"}, Range: []float64{0, 100}, Unit: "absolute", ScaleBy: "row"}}}},
				},
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_PivotFormatValidated(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:   []PivotField{{Field: "region"}},
					Values: []PivotValue{{Field: "sales", Format: []FormatLayer{{If: "bogus_op", Value: 1}}}},
				},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot.values[0].format[0].if: unknown operator")
}

func TestValidate_PivotScaleBy(t *testing.T) {
	pivotWith := func(l FormatLayer) *Dashboard {
		return &Dashboard{
			Name: "test",
			Rows: []Row{{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:   []PivotField{{Field: "region"}},
					Values: []PivotValue{{Field: "sales", Format: []FormatLayer{l}}},
				},
			}}}},
		}
	}
	// row/column on a gradient is fine.
	assertNoErr(t, Validate(pivotWith(FormatLayer{BackgroundColor: []interface{}{"red", "green"}, ScaleBy: "column"})))
	// An unknown value is rejected.
	assertValidationContains(t, Validate(pivotWith(FormatLayer{BackgroundColor: []interface{}{"red", "green"}, ScaleBy: "diagonal"})), "scaleBy: unknown value")
	// scaleBy on a flat (non-gradient) fill is rejected.
	assertValidationContains(t, Validate(pivotWith(FormatLayer{BackgroundColor: "green", ScaleBy: "row"})), "scaleBy: requires a gradient")
	// scaleBy on an ordinary table column is rejected — it's pivot-only.
	tbl := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeTable, SQL: "SELECT sales FROM orders",
		Columns: []TableColumn{{Name: "sales", Format: []FormatLayer{{BackgroundColor: []interface{}{"red", "green"}, ScaleBy: "row"}}}},
	}}}}}
	assertValidationContains(t, Validate(tbl), "scaleBy: only valid on pivot value formats")
}

func TestValidate_TableColumnBorder(t *testing.T) {
	tbl := func(border string) *Dashboard {
		return &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
			Name: "w", Type: WidgetTypeTable, SQL: "SELECT sales FROM orders",
			Columns: []TableColumn{{Name: "sales", Border: border}},
		}}}}}
	}
	for _, v := range []string{"left", "right", "both"} {
		assertNoErr(t, Validate(tbl(v)))
	}
	assertValidationContains(t, Validate(tbl("top")), "border: must be left, right, or both")

	// border is table-only — rejected on pivot_table widgets.
	pivot := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
		Pivot:   &PivotConfig{Rows: []PivotField{{Field: "region"}}, Values: []PivotValue{{Field: "sales"}}},
		Columns: []TableColumn{{Name: "sales", Border: "left"}},
	}}}}}
	assertValidationContains(t, Validate(pivot), "border: not supported on pivot tables")
}

func TestValidate_TableColumnFrozen(t *testing.T) {
	// frozen is accepted on plain tables.
	tbl := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeTable, SQL: "SELECT sales FROM orders",
		Columns: []TableColumn{{Name: "sales", Frozen: true}},
	}}}}}
	assertNoErr(t, Validate(tbl))

	// frozen is table-only — rejected on pivot_table widgets.
	pivot := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
		Pivot:   &PivotConfig{Rows: []PivotField{{Field: "region"}}, Values: []PivotValue{{Field: "sales"}}},
		Columns: []TableColumn{{Name: "sales", Frozen: true}},
	}}}}}
	assertValidationContains(t, Validate(pivot), "frozen: not supported on pivot tables")
}

func TestValidate_ImageWidget(t *testing.T) {
	// Data-driven: a query source plus src naming the image URL column.
	ok := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeImage, SQL: "SELECT photo, address FROM listings", Src: "photo", Title: "address", Fit: "cover",
	}}}}}
	assertNoErr(t, Validate(ok))

	// src (the image URL column) is required.
	noSrc := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeImage, SQL: "SELECT photo FROM listings",
	}}}}}
	assertValidationContains(t, Validate(noSrc), "src (image URL column) is required for image widgets")

	// No query source → error (data-driven, like a table).
	noSource := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeImage, Src: "photo",
	}}}}}
	if Validate(noSource) == nil {
		t.Fatal("expected an error for an image widget without a query source")
	}

	// fit must be contain or cover.
	badFit := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeImage, SQL: "SELECT photo FROM listings", Src: "photo", Fit: "stretch",
	}}}}}
	assertValidationContains(t, Validate(badFit), "fit must be contain or cover")
}

func TestValidate_TableColumnType(t *testing.T) {
	// type: image is accepted on plain tables.
	tbl := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeTable, SQL: "SELECT photo FROM listings",
		Columns: []TableColumn{{Name: "photo", Type: "image"}},
	}}}}}
	assertNoErr(t, Validate(tbl))

	// an unknown type is rejected.
	bad := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypeTable, SQL: "SELECT photo FROM listings",
		Columns: []TableColumn{{Name: "photo", Type: "video"}},
	}}}}}
	assertValidationContains(t, Validate(bad), "type: must be text or image")

	// type: image is table-only — rejected on pivot_table widgets.
	pivot := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, photo FROM listings",
		Pivot:   &PivotConfig{Rows: []PivotField{{Field: "region"}}, Values: []PivotValue{{Field: "photo"}}},
		Columns: []TableColumn{{Name: "photo", Type: "image"}},
	}}}}}
	assertValidationContains(t, Validate(pivot), "type: image not supported on pivot tables")

	// type: text is the default rendering and stays valid on pivot tables.
	pivotText := &Dashboard{Name: "test", Rows: []Row{{Widgets: []Widget{{
		Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
		Pivot:   &PivotConfig{Rows: []PivotField{{Field: "region"}}, Values: []PivotValue{{Field: "sales"}}},
		Columns: []TableColumn{{Name: "sales", Type: "text"}},
	}}}}}
	assertNoErr(t, Validate(pivotText))
}

func TestValidate_PivotOnlyOnTables(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypeChart, Chart: "line", SQL: "SELECT 1",
				X:     &AxisEncoding{Field: "month"},
				Y:     &AxisEncoding{Field: []string{"revenue"}},
				Pivot: &PivotConfig{Values: []PivotValue{{Field: "revenue"}}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot is only valid on pivot_table widgets")
}

func TestValidate_PivotRequiresValues(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region FROM orders",
				Pivot: &PivotConfig{Rows: []PivotField{{Field: "region"}}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot.values must list at least one value")
}

func TestValidate_PivotFieldOnBothAxes(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:    []PivotField{{Field: "region"}},
					Columns: []PivotField{{Field: "region"}},
					Values:  []PivotValue{{Field: "sales"}},
				},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "can't be in both rows and columns")
}

func TestValidate_PivotDuplicateFieldInAxis(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:   []PivotField{{Field: "region"}, {Field: "region"}},
					Values: []PivotValue{{Field: "sales"}},
				},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "is listed more than once")
}

func TestValidate_PivotRequiresPivotType(t *testing.T) {
	// A pivot config on a plain table is rejected — it must be a pivot_table.
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "w", Type: WidgetTypeTable, SQL: "SELECT region, sales FROM orders",
			Pivot: &PivotConfig{Values: []PivotValue{{Field: "sales"}}},
		}}}},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot is only valid on pivot_table widgets")
}

func TestValidate_PivotTableNeedsPivot(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{{Widgets: []Widget{{
			Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region FROM orders",
		}}}},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot_table widgets need a pivot")
}

func TestValidate_PivotInvalidSummarize(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT sales FROM orders",
				Pivot: &PivotConfig{Values: []PivotValue{{Field: "sales", Summarize: "total"}}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "summarize \"total\" is invalid")
}

func TestValidate_PivotInvalidOrder(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "w", Type: WidgetTypePivotTable, SQL: "SELECT region, sales FROM orders",
				Pivot: &PivotConfig{
					Rows:   []PivotField{{Field: "region", Order: "ascending"}},
					Values: []PivotValue{{Field: "sales"}},
				},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "pivot.rows[0].order must be asc or desc")
}

func TestValidate_FilterTypes(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Filters: []Filter{
			{Name: "region", Type: "select"},
			{Name: "date_range", Type: "date-range"},
			{Name: "as_of_date", Type: "date"},
			{Name: "min_revenue", Type: "number"},
			{Name: "search", Type: "text"},
		},
		Rows: []Row{
			{Widgets: []Widget{{Name: "w", Type: WidgetTypeText, Content: "hi"}}},
		},
	}
	assertNoErr(t, Validate(d))

	d.Filters = append(d.Filters, Filter{Name: "bad", Type: "boolean"})
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, `unknown filter type "boolean"`)
}

func TestValidate_ProjectSemanticDashboard(t *testing.T) {
	d, err := LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	assertNoErr(t, err)

	err = Validate(d)
	assertNoErr(t, err)
}

func TestValidate_ProjectSemanticDashboardMissingModel(t *testing.T) {
	d, err := LoadFile("../../testdata/project/dashboards/semantic-sales.yml")
	assertNoErr(t, err)

	d.Model = "missing_model"
	err = Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "semantic model \"missing_model\" not found")
}

func TestValidate_InvalidExternalSemanticModelOnlyFailsReferencedDashboard(t *testing.T) {
	projectDir := t.TempDir()

	dashboardsDir := filepath.Join(projectDir, "dashboards")
	semanticDir := filepath.Join(projectDir, "semantic")
	assertNoErr(t, os.MkdirAll(dashboardsDir, 0o755))
	assertNoErr(t, os.MkdirAll(semanticDir, 0o755))

	regularDashboard := `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Regular Dashboard
rows:
  - widgets:
      - name: Notes
        type: text
        content: Hello
`
	semanticDashboard := `schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Semantic Dashboard
model: broken_sales
rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
`
	invalidModel := `schema: v1
name: broken_sales
source:
  table: sales
dimensions:
  - name: revenue
    type: string
metrics:
  - name: revenue
    expression: sum(amount)
`

	assertNoErr(t, os.WriteFile(filepath.Join(dashboardsDir, "regular.yml"), []byte(regularDashboard), 0o644))
	assertNoErr(t, os.WriteFile(filepath.Join(dashboardsDir, "semantic.yml"), []byte(semanticDashboard), 0o644))
	assertNoErr(t, os.WriteFile(filepath.Join(semanticDir, "broken-sales.yml"), []byte(invalidModel), 0o644))

	dashboards, err := LoadDir(projectDir)
	assertNoErr(t, err)

	regular := FindByName(dashboards, "Regular Dashboard")
	if regular == nil {
		t.Fatal("expected regular dashboard to load")
	}
	assertNoErr(t, Validate(regular))

	semanticDash := FindByName(dashboards, "Semantic Dashboard")
	if semanticDash == nil {
		t.Fatal("expected semantic dashboard to load")
	}

	err = Validate(semanticDash)
	assertErr(t, err)
	assertValidationContains(t, err, `semantic model "broken_sales" is invalid`)
}

// ---------------------------------------------------------------------------
// Inline data widgets
// ---------------------------------------------------------------------------

func TestValidate_InlineDataTableNoQuery(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Recent", Type: WidgetTypeTable, Col: 12,
				Data: &WidgetData{
					Columns: []string{"id", "amount"},
					Rows:    [][]any{{1, 10.5}, {2, 20}},
				},
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_InlineDataChart(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Revenue", Type: WidgetTypeChart, Chart: "bar", Col: 6,
				Data: &WidgetData{
					Columns: []string{"quarter", "revenue"},
					Rows:    [][]any{{"Q1", 100}, {"Q2", 200}},
				},
				X: newAxisField("quarter"),
				Y: newAxisFields([]string{"revenue"}),
			}}},
		},
	}
	assertNoErr(t, Validate(d))
}

func TestValidate_InlineDataRowArityMismatch(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Recent", Type: WidgetTypeTable, Col: 12,
				Data: &WidgetData{
					Columns: []string{"id", "amount"},
					Rows:    [][]any{{1}},
				},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "expected 2 to match columns")
}

func TestValidate_InlineDataConflictsWithSQL(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Recent", Type: WidgetTypeTable, Col: 12,
				SQL:  "SELECT 1",
				Data: &WidgetData{Columns: []string{"id"}, Rows: [][]any{{1}}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "data cannot be combined with sql")
}

func TestValidate_InlineDataInvalidOnText(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Notes", Type: WidgetTypeText, Content: "hi",
				Data: &WidgetData{Columns: []string{"id"}, Rows: [][]any{{1}}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "data is only valid on metric, chart, table, pivot_table, or image widgets")
}

func TestValidate_InlineDataEmptyColumns(t *testing.T) {
	d := &Dashboard{
		Name: "test",
		Rows: []Row{
			{Widgets: []Widget{{
				Name: "Recent", Type: WidgetTypeTable, Col: 12,
				Data: &WidgetData{Columns: []string{}, Rows: [][]any{}},
			}}},
		},
	}
	err := Validate(d)
	assertErr(t, err)
	assertValidationContains(t, err, "data.columns is required and must be non-empty")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertValidationContains(t *testing.T, err error, substr string) {
	t.Helper()
	var ve *ValidationError
	if errors.As(err, &ve) {
		for _, e := range ve.Errors {
			if strings.Contains(e, substr) {
				return
			}
		}
		t.Errorf("expected validation error containing %q, got errors: %v", substr, ve.Errors)
		return
	}
	if strings.Contains(err.Error(), substr) {
		return
	}
	t.Errorf("expected error containing %q, got: %v", substr, err)
}
