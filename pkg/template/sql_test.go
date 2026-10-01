package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderSQL(t *testing.T) {
	tests := []struct {
		name, source, want string
		values             map[string]any
	}{
		{"date", "SELECT DATE('{{ filters.date }}')", "SELECT DATE('2024-10-01')", map[string]any{"date": "2024-10-01"}},
		{"text", "SELECT * FROM t WHERE name LIKE '%{{ filters.search }}%'", "SELECT * FROM t WHERE name LIKE '%hello world; --%'", map[string]any{"search": "hello world; --"}},
		{"number", "SELECT {{ filters.n }}", "SELECT  12.5 ", map[string]any{"n": 12.5}},
		{"negative number token boundary", "SELECT -{{ filters.n }}", "SELECT - -1 ", map[string]any{"n": -1}},
		{"conditional", "SELECT 1 {% if filters.region != 'All' %}WHERE r = '{{ filters.region }}'{% endif %}", "SELECT 1 WHERE r = 'EU'", map[string]any{"region": "EU"}},
		{"conditional skipped", "SELECT 1 {% if filters.region != 'All' %}WHERE r = '{{ filters.region }}'{% endif %}", "SELECT 1 ", map[string]any{"region": "All"}},
		{"bracket reference", "SELECT '{{ filters['region'] }}'", "SELECT 'EU'", map[string]any{"region": "EU"}},
		{"repeated reference", "SELECT '{{ filters.x }}', '{{ filters.x }}'", "SELECT 'yes', 'yes'", map[string]any{"x": "yes"}},
		{"join", `SELECT * FROM t WHERE x IN ('{{ filters.x | join("','") }}')`, "SELECT * FROM t WHERE x IN ('ios','android')", map[string]any{"x": []any{"ios", "android"}}},
		{"loop", `SELECT {% for x in filters.x %}'{{ x }}'{% if not loop.last %},{% endif %}{% endfor %}`, "SELECT 'ios','android'", map[string]any{"x": []any{"ios", "android"}}},
		{"trim", "SELECT ' {{- filters.x -}} '", "SELECT 'yes'", map[string]any{"x": "yes"}},
		{"string containing Jinja delimiter", `SELECT '{{ filters.x | default("}}") }}'`, "SELECT 'yes'", map[string]any{"x": "yes"}},
		{"unicode", "SELECT 'İstanbul {{ filters.x }}'", "SELECT 'İstanbul Türkiye'", map[string]any{"x": "Türkiye"}},
		{"backslash without outputs", `SELECT regexp_replace(p, '\D', '') {% if filters.x %}WHERE 1 = 1{% endif %}`, `SELECT regexp_replace(p, '\D', '') WHERE 1 = 1`, map[string]any{"x": "yes"}},
		{"backslash after last output", `SELECT '{{ filters.x }}', regexp_replace(p, '\D', '')`, `SELECT 'yes', regexp_replace(p, '\D', '')`, map[string]any{"x": "yes"}},
		{"postgres json path", "SELECT * FROM t WHERE data #>> '{a,b}' = '{{ filters.x }}'", "SELECT * FROM t WHERE data #>> '{a,b}' = 'yes'", map[string]any{"x": "yes"}},
		{"array literal", "SELECT * FROM t WHERE r = ANY(ARRAY['{{ filters.x }}'])", "SELECT * FROM t WHERE r = ANY(ARRAY['yes'])", map[string]any{"x": "yes"}},
		{"list literal", `SELECT ['{{ filters.x | join("','") }}']`, "SELECT ['ios','android']", map[string]any{"x": []any{"ios", "android"}}},
		{"nested list before output", "SELECT [[1, 2], [3]] AS l WHERE x = '{{ filters.x }}'", "SELECT [[1, 2], [3]] AS l WHERE x = 'yes'", map[string]any{"x": "yes"}},
		{"array before output", "SELECT * FROM t WHERE r = ANY(ARRAY['a','b']) AND x = '{{ filters.x }}'", "SELECT * FROM t WHERE r = ANY(ARRAY['a','b']) AND x = 'yes'", map[string]any{"x": "yes"}},
		{"json path before output", "SELECT data #>> '{a}' AS a\nFROM t WHERE x = '{{ filters.x }}'", "SELECT data #>> '{a}' AS a\nFROM t WHERE x = 'yes'", map[string]any{"x": "yes"}},
		{"bracket identifier before output", "SELECT [it's] FROM t WHERE r = '{{ filters.x }}'", "SELECT [it's] FROM t WHERE r = 'yes'", map[string]any{"x": "yes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RenderSQL(tt.source, tt.values)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderSQLRejectsUnsafeOutputs(t *testing.T) {
	tests := []struct {
		name, source string
		value        any
	}{
		{"reported payload", "SELECT DATE('{{ filters.x }}')", "2024-10-01'--"},
		{"quote", "SELECT '{{ filters.x }}'", "O'Reilly"},
		{"backslash", "SELECT '{{ filters.x }}'", `a\`},
		{"control", "SELECT '{{ filters.x }}'", "a\x00b"},
		{"unquoted SQL", "SELECT 1 WHERE {{ filters.x }}", "true UNION SELECT secret FROM private"},
		{"identifier", "SELECT * FROM {{ filters.x }}", "private"},
		{"quoted identifier", `SELECT "{{ filters.x }}" FROM t`, "private"},
		{"backtick", "SELECT `{{ filters.x }}` FROM t", "private"},
		{"bracket identifier", "SELECT [{{ filters.x }}] FROM t", "private"},
		{"dollar quote", "SELECT $$'{{ filters.x }}'$$", "private"},
		{"tagged dollar quote", "SELECT $body$'{{ filters.x }}'$body$", "private"},
		{"unicode dollar quote", "SELECT $é$'{{ filters.x }}'$é$", "$é$ UNION SELECT secret FROM private --"},
		{"triple quote", "SELECT '''{{ filters.x }}'''", "private"},
		{"line comment", "SELECT 1 -- {{ filters.x }}", "private"},
		{"block comment", "SELECT 1 /* {{ filters.x }} */", "private"},
		{"nested block comment", "SELECT 1 /* /* */ {{ filters.x }} */", "private"},
		{"numeric comment", "SELECT 1 -- {{ filters.x }}", 12},
		{"numeric string", "SELECT {{ filters.x }}", "12"},
		{"array injection", `SELECT '{{ filters.x | join("','") }}'`, []any{"ios", "x') OR 1=1 --"}},
		{"transformed output", `SELECT '{{ filters.x | replace("X", "'") }}'`, "X OR 1=1 --"},
		{"escaped template literal", `SELECT '\'{{ filters.x }}'`, "x"},
		{"hash comment", "SELECT 1 # it's {{ filters.x }}", "private"},
		{"hash arrow comment", "SELECT * FROM t #> don't\nWHERE name = {{ filters.x }} -- '", "1=1 OR true"},
		{"tsql bracket identifier", "SELECT [1x'] , {{ filters.x }} , ']'", "1=1 OR true"},
		{"tsql escaped bracket identifier", "SELECT [1x]]'] , {{ filters.x }} , ''", "1=1 OR true"},
		{"tsql bracket identifier number", "SELECT [1st Year's Revenue] FROM t WHERE x = '{{ filters.x }}'", "private"},
		{"filter block", `SELECT 1 WHERE {% filter replace("X", filters.x) %}X{% endfilter %}`, "1=1 OR true"},
		{"transformed set block", `{% set y %}{{ filters.x }}{% endset %}SELECT '{{ y | upper }}'`, "private"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, err := RenderSQL(tt.source, map[string]any{"x": tt.value})
			if err == nil {
				t.Fatalf("unsafe query accepted: %q", sql)
			}
		})
	}
}

func TestRenderSQLCannotIncludeUncapturedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.sql")
	if err := os.WriteFile(path, []byte("SELECT {{ filters.x }}"), 0600); err != nil {
		t.Fatal(err)
	}
	if sql, err := RenderSQL("{% include filters.file %}", map[string]any{"file": path, "x": "secret FROM private"}); err == nil {
		t.Fatalf("included unprotected SQL: %s", sql)
	}
}
