package slides

import (
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/server"
)

func TestFormatSemanticNumber(t *testing.T) {
	two := 2
	cases := []struct {
		value  float64
		format dashboard.SemanticFormat
		want   string
	}{
		{-1234.5, dashboard.SemanticFormat{Currency: "TRY"}, "-₺1,234.50"},
		{-1234.5, dashboard.SemanticFormat{Currency: "SEK"}, "-kr 1,234.50"},
		{1234.5, dashboard.SemanticFormat{Currency: "EGP"}, "E£1,234.50"},
		// Newer ISO codes missing from x/text keep the code, grouped and rounded.
		{-1234.567, dashboard.SemanticFormat{Currency: "VES"}, "-VES 1,234.57"},
		{0.1234, dashboard.SemanticFormat{Type: "percentage", Decimals: &two}, "12.34%"},
		{1234.56789, dashboard.SemanticFormat{Type: "number"}, "1,234.568"},
	}
	for _, c := range cases {
		if got, _ := formatSemanticNumber(c.value, c.format, false); got != c.want {
			t.Errorf("formatSemanticNumber(%v, %+v) = %q, want %q", c.value, c.format, got, c.want)
		}
	}
}

// Chart ticks match the browser's Intl compact notation (three significant digits).
func TestFormatSemanticNumberCompact(t *testing.T) {
	two := 2
	cases := []struct {
		value  float64
		format dashboard.SemanticFormat
		want   string
	}{
		{1500000, dashboard.SemanticFormat{Currency: "TRY", Decimals: &two}, "₺1.5M"},
		{-1500000, dashboard.SemanticFormat{Currency: "TRY"}, "-₺1.5M"},
		{1500000, dashboard.SemanticFormat{Currency: "CHF"}, "CHF 1.5M"},
		{1234, dashboard.SemanticFormat{Type: "number"}, "1.23K"},
		{1125000, dashboard.SemanticFormat{Type: "number"}, "1.13M"},
		{999999, dashboard.SemanticFormat{Type: "number"}, "1M"},
		{999.5, dashboard.SemanticFormat{Type: "number"}, "1K"},
		{0.00123456, dashboard.SemanticFormat{Type: "number"}, "0.00123"},
		{1e16, dashboard.SemanticFormat{Type: "number"}, "10,000T"},
		{0, dashboard.SemanticFormat{Type: "number"}, "0"},
		{0.125, dashboard.SemanticFormat{Type: "percentage"}, "12.5%"},
	}
	for _, c := range cases {
		if got, _ := formatSemanticNumber(c.value, c.format, true); got != c.want {
			t.Errorf("formatSemanticNumber(%v, %+v, compact) = %q, want %q", c.value, c.format, got, c.want)
		}
	}
}

// Sparkline cells format their latest point with y.format before `number`.
func TestSparklineValueUsesYFormat(t *testing.T) {
	column := dashboard.TableColumn{
		Name:   "trend",
		Type:   "sparkline",
		Number: "number",
		X:      &dashboard.AxisEncoding{Field: "d"},
		Y:      &dashboard.AxisEncoding{Field: "v", Format: "$,.0f"},
	}
	w := &dashboard.Widget{Name: "t", Type: dashboard.WidgetTypeTable, Columns: []dashboard.TableColumn{column}}
	data := &server.WidgetQueryResult{Rows: [][]any{{`[{"d":"2024-01-01","v":1500}]`}}}
	data.Columns = append(data.Columns, struct {
		Name string `json:"name"`
		Type string `json:"type,omitempty"`
	}{Name: "trend"})
	var found bool
	for _, req := range tableReqs("p", "s", w, data, 0, 0, 100) {
		if req.InsertText != nil && req.InsertText.Text == "$1,500" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected sparkline cell text $1,500")
	}
}

func TestFormatTableValueNumberCapsDecimals(t *testing.T) {
	if got := formatTableValue(0.1+0.2, dashboard.TableColumn{Name: "x", Number: "number"}); got != "0.3" {
		t.Fatalf("got %q, want 0.3", got)
	}
}
