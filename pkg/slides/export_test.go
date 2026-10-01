package slides

import "testing"

func TestSparklineLatest(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"objects", []any{map[string]any{"revenue": 10}, map[string]any{"revenue": 12.5}}, "12.5"},
		{"pairs", []any{[]any{"2026-09-01", 3}, []any{"2026-09-02", 4}}, "4"},
		{"skips trailing null", []any{map[string]any{"revenue": 7}, map[string]any{"revenue": nil}}, "7"},
		{"native large number", []any{map[string]any{"revenue": float64(1500000)}}, "1500000"},
		{"skips trailing non-numeric", []any{map[string]any{"revenue": 7}, map[string]any{"revenue": ""}, map[string]any{"revenue": "abc"}}, "7"},
		{"numeric string", []any{[]any{"2026-09-01", " 42 "}}, "42"},
		{"empty", []any{}, "—"},
		{"not an array", "oops", "—"},
		{"json objects", `[{"revenue": 10}, {"revenue": 12.5}]`, "12.5"},
		{"json pairs", " \uFEFF[[\"2026-09-01\", 3], [\"2026-09-02\", 4]] ", "4"},
		{"json large number", `[{"revenue": 1500000}]`, "1500000"},
		{"json non-array", `{"revenue": 10}`, "—"},
		{"invalid json", "[oops", "—"},
		{"json trailing data", `[{"revenue": 10}] [1]`, "—"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := sparklineLatest(tt.value, "revenue"); got != tt.want {
				t.Fatalf("sparklineLatest() = %q, want %q", got, tt.want)
			}
		})
	}
}
