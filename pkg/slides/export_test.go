package slides

import "testing"

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
