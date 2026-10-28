package slides

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/server"
)

func TestSplitSlideRows(t *testing.T) {
	cases := []struct {
		name string
		cols []int
		want [][]int
	}{
		{"equal halves", []int{6, 6}, [][]int{{6, 6}}},
		{"sidebar", []int{8, 4}, [][]int{{8, 4}}},
		{"two wide cards", []int{8, 8}, [][]int{{8}, {8}}},
		{"fill continuation", []int{8, 8, 4}, [][]int{{8}, {8, 4}}},
		{"full width", []int{12, 12, 12}, [][]int{{12}, {12}, {12}}},
		{"implicit widths", []int{0, 0, 0}, [][]int{{4, 4, 4}}},
		{"mixed widths", []int{8, 0}, [][]int{{8}, {6}}},
		{"many implicit widths", make([]int, 13), [][]int{{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, {1}}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			row := dashboard.Row{Tab: "Overview", Height: 320}
			for i, col := range tt.cols {
				row.Widgets = append(row.Widgets, dashboard.Widget{ID: fmt.Sprint(i), Col: col})
			}
			var got [][]int
			offset := 0
			for _, slide := range splitSlideRows([]dashboard.Row{row}) {
				if slide.rowIndex != 0 || slide.widgetOffset != offset || slide.row.Tab != row.Tab || slide.row.Height != row.Height {
					t.Fatalf("slide lost original row metadata or widget offset: %+v", slide)
				}
				var cols []int
				for _, w := range slide.row.Widgets {
					if w.ID != row.Widgets[offset].ID {
						t.Fatalf("widget order changed at %d", offset)
					}
					cols = append(cols, w.Col)
					offset++
				}
				got = append(got, cols)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("slide widths = %v, want %v", got, tt.want)
			}
			for i, w := range row.Widgets {
				if w.Col != tt.cols[i] {
					t.Fatal("splitting slides mutated the dashboard")
				}
			}
		})
	}
}

func TestWrappedWidgetsStayOnSlideAndKeepQueryResults(t *testing.T) {
	d := &dashboard.Dashboard{Rows: []dashboard.Row{
		{Widgets: []dashboard.Widget{{Type: dashboard.WidgetTypeMetric, Col: 12}}},
		{Widgets: []dashboard.Widget{
			{Name: "First", Type: dashboard.WidgetTypeMetric, Col: 8},
			{Name: "Second", Type: dashboard.WidgetTypeMetric, Col: 8},
			{Name: "Tabbed", Col: 8, Tabs: []dashboard.Widget{{Name: "Details", Type: dashboard.WidgetTypeMetric}}},
		}},
	}}
	data := map[string]*server.WidgetQueryResult{
		server.WidgetID(1, 0):               {Rows: [][]any{{"first result"}}},
		server.WidgetID(1, 1):               {Rows: [][]any{{"second result"}}},
		server.WidgetTabID(1, 2, "Details"): {Rows: [][]any{{"tab result"}}},
	}
	want := []string{"first result", "second result", "tab result"}
	slides := splitSlideRows(d.Rows)
	if len(slides) != 4 {
		t.Fatalf("got %d slides, want 4", len(slides))
	}
	for j, slide := range slides[1:] {
		reqs, _, err := widgetRequests(context.Background(), nil, fmt.Sprintf("slide_%d", j+1), slide, d, data)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, req := range reqs {
			if shape := req.CreateShape; shape != nil {
				p := shape.ElementProperties
				if p.Transform.TranslateX < float64(padX) || p.Transform.TranslateX+p.Size.Width.Magnitude > float64(padX+contentW) ||
					p.Transform.TranslateY < float64(padY) || p.Transform.TranslateY+p.Size.Height.Magnitude > float64(padY+contentH) {
					t.Errorf("shape %s extends outside slide content: %+v", shape.ObjectId, p)
				}
			}
			if text := req.InsertText; text != nil && text.ObjectId == fmt.Sprintf("r1_w%d_value", j) {
				found = true
				if text.Text != want[j] {
					t.Errorf("widget %d result = %q, want %q", j, text.Text, want[j])
				}
			}
		}
		if !found {
			t.Errorf("missing original widget %d value on continuation slide", j)
		}
	}
}

func TestSparklineLatest(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"objects", []any{map[string]any{"day": "d", "revenue": 10}, map[string]any{"day": "d", "revenue": 12.5}}, "12.5"},
		{"pairs", []any{[]any{"2026-09-01", 3}, []any{"2026-09-02", 4}}, "4"},
		{"skips trailing null", []any{map[string]any{"day": "d", "revenue": 7}, map[string]any{"day": "d", "revenue": nil}}, "7"},
		{"native large number", []any{map[string]any{"day": "d", "revenue": float64(1500000)}}, "1500000"},
		{"skips trailing non-numeric", []any{map[string]any{"day": "d", "revenue": 7}, map[string]any{"day": "d", "revenue": ""}, map[string]any{"day": "d", "revenue": "abc"}}, "7"},
		{"numeric string", []any{[]any{"2026-09-01", " 42 "}}, "42"},
		{"empty", []any{}, "—"},
		{"not an array", "oops", "—"},
		{"json objects", `[{"day": "d", "revenue": 10}, {"day": "d", "revenue": 12.5}]`, "12.5"},
		{"json pairs", " \uFEFF[[\"2026-09-01\", 3], [\"2026-09-02\", 4]] ", "4"},
		{"json large number", `[{"day": "d", "revenue": 1500000}]`, "1500000"},
		{"json non-array", `{"day": "d", "revenue": 10}`, "—"},
		{"invalid json", "[oops", "—"},
		{"skips missing x", []any{map[string]any{"day": "d", "revenue": 7}, map[string]any{"revenue": 9}}, "7"},
		{"skips null x pair", []any{[]any{"2026-09-01", 3}, []any{nil, 5}}, "3"},
		{"numeric x", []any{[]any{1, 3}, []any{2, 4}}, "4"},
		{"json trailing data", `[{"day": "d", "revenue": 10}] [1]`, "—"},
	}
	// Downsampling to 100 points always keeps the last point.
	long := make([]any, 250)
	for i := range long {
		long[i] = []any{i, i}
	}
	cases = append(cases, struct {
		name  string
		value any
		want  string
	}{"downsampled", long, "249"})
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := sparklineLatest(tt.value, "day", "revenue"); got != tt.want {
				t.Fatalf("sparklineLatest() = %q, want %q", got, tt.want)
			}
		})
	}
}
