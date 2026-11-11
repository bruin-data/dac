package template

import (
	"os"
	"path/filepath"
	"strings"
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
		{"positional column", "SELECT $1, t.$2 FROM @stage t WHERE $3 = '{{ filters.x }}'", "SELECT $1, t.$2 FROM @stage t WHERE $3 = 'yes'", map[string]any{"x": "yes"}},
		{"dollar in identifier", "SELECT my$col FROM t WHERE x = '{{ filters.x }}'", "SELECT my$col FROM t WHERE x = 'yes'", map[string]any{"x": "yes"}},
		{"dollar quote after output", "SELECT {{ filters.n }}$$a$$, '{{ filters.x }}'", "SELECT  1 $$a$$, 'yes'", map[string]any{"n": 1, "x": "yes"}},
		{"array before output", "SELECT * FROM t WHERE r = ANY(ARRAY['a','b']) AND x = '{{ filters.x }}'", "SELECT * FROM t WHERE r = ANY(ARRAY['a','b']) AND x = 'yes'", map[string]any{"x": "yes"}},
		{"json path before output", "SELECT data #>> '{a}' AS a\nFROM t WHERE x = '{{ filters.x }}'", "SELECT data #>> '{a}' AS a\nFROM t WHERE x = 'yes'", map[string]any{"x": "yes"}},
		{"bracket identifier before output", "SELECT [Revenue] FROM t WHERE r = '{{ filters.x }}'", "SELECT [Revenue] FROM t WHERE r = 'yes'", map[string]any{"x": "yes"}},
		{"comments before output", "-- don't\n# note\n/* it's */ SELECT 1 WHERE r = '{{ filters.x }}'", "-- don't\n# note\n/* it's */ SELECT 1 WHERE r = 'yes'", map[string]any{"x": "yes"}},
		{"doubled quote literal", "SELECT replace(a, '''', '') FROM t WHERE r = '{{ filters.x }}'", "SELECT replace(a, '''', '') FROM t WHERE r = 'yes'", map[string]any{"x": "yes"}},
		{"triple quote", "SELECT '''{{ filters.x }}'''", "SELECT '''yes'''", map[string]any{"x": "yes"}},
		{"integer division", "SELECT a // 2 FROM t WHERE r = '{{ filters.x }}' AND n = {{ filters.n }} // 3", "SELECT a // 2 FROM t WHERE r = 'yes' AND n =  1  // 3", map[string]any{"x": "yes", "n": 1}},
		{"session variable", "SELECT * FROM t WHERE v = $myvar AND a = '{{ filters.x }}'", "SELECT * FROM t WHERE v = $myvar AND a = 'yes'", map[string]any{"x": "yes"}},
		// Jinja reads "\r\n" as "\n", so rendered SQL has normalized line endings.
		{"crlf comment", "-- note\r\nSELECT 1 WHERE r = '{{ filters.x }}'", "-- note\nSELECT 1 WHERE r = 'yes'", map[string]any{"x": "yes"}},
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
		{"digit bracket identifier", "SELECT revenue AS [2024 '{{ filters.x }}'] FROM t", "] FROM t; SELECT secret FROM private --"},
		{"quote inside dollar quote", "SELECT $$ ' $$ AS a, '{{ filters.x }}' AS b", ", (SELECT secret FROM private) #"},
		{"quote inside tagged dollar quote", "SELECT $q$ ' $q$ AS a, '{{ filters.x }}' AS b", ", (SELECT secret FROM private) #"},
		{"spaced bracket identifier", "SELECT revenue AS [ '{{ filters.x }}' ] FROM t", "] FROM t; SELECT secret FROM private --"},
		{"dollar quote", "SELECT $$'{{ filters.x }}'$$", "private"},
		{"tagged dollar quote", "SELECT $body$'{{ filters.x }}'$body$", "private"},
		{"unicode dollar quote", "SELECT $é$'{{ filters.x }}'$é$", "$é$ UNION SELECT secret FROM private --"},
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
		{"bracket identifier with quote", "SELECT [it's] FROM t WHERE r = '{{ filters.x }}'", "private"},
		{"duckdb list read as identifier", "SELECT [x, ']'] AS l FROM t WHERE id = {{ filters.x }}", "1=1 OR true"},
		{"bracket identifier hiding comment", "SELECT [x -- ] '\n{{ filters.x }}'", "private"},
		{"non-nesting dialect comment", "/* /* */ SELECT ' */ ' AS a FROM t WHERE id = {{ filters.x }}", "1=1 OR true"},
		{"non-nesting dialect hash", "/* /* */ # */ '\n{{ filters.x }}'", "private"},
		{"mysql executable comment", "SELECT /*! ' */ '{{ filters.x }}'", "private"},
		{"mysql dash dash operator", "SELECT a--'x\n' FROM t WHERE id = {{ filters.x }}", "1=1 OR true"},
		{"postgres hash operator", "SELECT 1 # '\nFROM t WHERE id = '{{ filters.x }}'", "private"},
		{"doubled quote read as triple quote", "SELECT replace(a, '''', '') AS a FROM t WHERE b <> '''' AND c = 'x' AND id = {{ filters.x }}", "1=1 OR true"},
		{"bare carriage return in comment", "SELECT 1 -- a\r'\nFROM t WHERE id = {{ filters.x }} -- '", "1=1 OR true"},
		{"bare carriage return in hash comment", "SELECT 1 # a\r'\nFROM t WHERE id = {{ filters.x }} -- '", "1=1 OR true"},
		{"snowflake slash comment", "SELECT * FROM t // customer's region\nWHERE region = {{ filters.x }}\nAND status = 'active'", "1=1 OR true"},
		{"dollar tag after name", "SELECT $a$'$a$ AND x = {{ filters.x }} -- '", "1=1 OR true"},
		{"filter block", `SELECT 1 WHERE {% filter replace("X", filters.x) %}X{% endfilter %}`, "1=1 OR true"},
		{"transformed set block", `{% set y %}{{ filters.x }}{% endset %}SELECT '{{ y | upper }}'`, "private"},
		// Offsets from a lexer that read each "\r\n" as "\n" once fell short of
		// the output, leaving it uncaptured, with the capture call in the comment.
		{"crlf lines before jinja comment", strings.Repeat("-- header\r\n", 20) + "SELECT * FROM t WHERE id = {# id from the filter bar #}{{ filters.x }}", "0 UNION SELECT secret FROM private"},
		{"crlf lines before output", strings.Repeat("-- header\r\n", 20) + "SELECT * FROM t WHERE id = {{ filters.x }}", "0 UNION SELECT secret FROM private"},
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

// newlineCases place an output after, inside and before line comments, block
// comments and literals that span a line ending, written as NL.
var newlineCases = []struct {
	name, sql string
	// safe: accepted with "\n" or "\r\n" line endings. Only "\n" ends a
	// line comment for every dialect, so a bare '\r' does not end one for
	// substituteSQLValues either; safeBareCR is its verdict for that text.
	safe, safeBareCR bool
}{
	{"after line comment", "-- c NL SELECT 'OUT'", true, false},
	{"after hash comment", "# c NL SELECT 'OUT'", true, false},
	{"inside line comment", "SELECT 1 -- OUT NL FROM t", false, false},
	{"before line comment", "SELECT 'OUT' -- c NL FROM t", true, true},
	{"after quote in line comment", "SELECT 1 -- a' NL FROM t WHERE id = OUT -- '", false, false},
	{"after block comment", "/* a NL b */ SELECT 'OUT'", true, true},
	{"inside block comment", "/* a NL OUT */ SELECT 1", false, false},
	{"before block comment", "SELECT 'OUT' /* a NL b */", true, true},
	{"after literal", "SELECT 'a NL b', 'OUT'", true, true},
	{"inside literal", "SELECT 'a NL OUT NL b'", true, true},
	{"before literal", "SELECT 'OUT', 'a NL b'", true, true},
	{"unquoted after line comment", "-- c NL SELECT OUT", false, false},
}

var newlines = []struct{ name, nl string }{{"lf", "\n"}, {"crlf", "\r\n"}, {"cr", "\r"}}

func newlineSQL(sql, nl, out string) string {
	return strings.NewReplacer(" NL ", nl, "OUT", out).Replace(sql)
}

// The capture step must wrap every output exactly. Its refusal of input with
// \r depends on gonja normalizing newlines (v2.7.0 did not); the guarantee is
// RenderSQL normalizing first, covered by TestRenderSQLLineEndings.
func TestCaptureSQLExpressionsLineEndings(t *testing.T) {
	for _, tt := range newlineCases {
		for _, nl := range newlines {
			t.Run(tt.name+"/"+nl.name, func(t *testing.T) {
				source := newlineSQL(tt.sql, nl.nl, "{{ filters.x }}") + "{{ filters.y }}"
				got, err := captureSQLExpressions(source, "C", "L")
				if nl.nl != "\n" {
					if err == nil {
						t.Fatalf("captured unnormalized source: %q", got)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				want := newlineSQL(tt.sql, nl.nl, "{{C(( filters.x ))}}") + "{{C(( filters.y ))}}"
				if got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

// The substitution step checks rendered SQL as the database will see it.
func TestSubstituteSQLValuesLineEndings(t *testing.T) {
	values := []sqlOutput{{text: "1=1 OR true"}}
	for _, tt := range newlineCases {
		for _, nl := range newlines {
			t.Run(tt.name+"/"+nl.name, func(t *testing.T) {
				got, err := substituteSQLValues(newlineSQL(tt.sql, nl.nl, "P_0END"), "P_", values)
				safe := tt.safe
				if nl.nl == "\r" {
					safe = tt.safeBareCR
				}
				if !safe {
					if err == nil {
						t.Fatalf("unsafe query accepted: %q", got)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if want := newlineSQL(tt.sql, nl.nl, "1=1 OR true"); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

// End to end, CRLF renders as LF, and a bare '\r' is rejected anywhere: the
// renderer turns it into '\n', hiding whether it ended a line comment.
func TestRenderSQLLineEndings(t *testing.T) {
	for _, tt := range newlineCases {
		for _, nl := range newlines {
			t.Run(tt.name+"/"+nl.name, func(t *testing.T) {
				got, err := RenderSQL(newlineSQL(tt.sql, nl.nl, "{{ filters.x }}"), map[string]any{"x": "1=1 OR true"})
				if !tt.safe || nl.nl == "\r" {
					if err == nil {
						t.Fatalf("query accepted: %q", got)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if want := newlineSQL(tt.sql, "\n", "1=1 OR true"); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestRenderSQLBareCarriageReturn(t *testing.T) {
	for _, source := range []string{
		"SELECT 'a\rb' WHERE x = '{{ filters.x }}'",
		"SELECT 1\r{% if filters.x %} WHERE 1 = 1{% endif %}",
		"SELECT '{{ filters.x }}'\r",
		"SELECT {# \r #} '{{ filters.x }}'",
	} {
		if sql, err := RenderSQL(source, map[string]any{"x": "yes"}); err == nil {
			t.Errorf("bare carriage return accepted in %q: %q", source, sql)
		}
	}
	// Plain SQL never reaches the Jinja lexer and is passed through verbatim.
	if got, err := RenderSQL("SELECT 1\r\n-- a\rb\n", nil); err != nil || got != "SELECT 1\r\n-- a\rb\n" {
		t.Errorf("plain SQL changed: %q, %v", got, err)
	}
}

func TestRenderSQLJinjaOperators(t *testing.T) {
	tests := []struct {
		name, source, want string
		values             map[string]any
	}{
		// gonja now returns the deciding operand, as Jinja2 does; it rendered
		// True or False before.
		{"or fallback", "SELECT '{{ filters.x or 'All' }}'", "SELECT 'All'", map[string]any{"x": ""}},
		{"or value", "SELECT '{{ filters.x or 'All' }}'", "SELECT 'EU'", map[string]any{"x": "EU"}},
		{"and value", "SELECT '{{ filters.x and 'set' }}'", "SELECT 'set'", map[string]any{"x": "EU"}},
		{"numeric or", "SELECT {{ filters.n or 10 }}", "SELECT  10 ", map[string]any{"n": 0}},
		{"loop nextitem", "SELECT {% for x in filters.x %}'{{ x }}'{% if loop.nextitem %},{% endif %}{% endfor %}", "SELECT 'a','b'", map[string]any{"x": []any{"a", "b"}}},
		// Unlike Jinja2's default, SQL keeps its trailing newline: the semantic
		// engine joins fragments with " AND ", after which a final line comment
		// would hide the next condition.
		{"trailing newline", "SELECT '{{ filters.x }}'\n", "SELECT 'EU'\n", map[string]any{"x": "EU"}},
		{"trailing line comment", "c = '{{ filters.x }}' -- from the filter bar\n", "c = 'EU' -- from the filter bar\n", map[string]any{"x": "EU"}},
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
	// An operand returned by 'or' is still captured and checked.
	if sql, err := RenderSQL("SELECT {{ filters.x or 1 }}", map[string]any{"x": "1 OR 1=1"}); err == nil {
		t.Errorf("unsafe operand accepted: %q", sql)
	}
}

func TestRenderSQLRecoversTemplatePanics(t *testing.T) {
	// gonja panics parsing an empty raw block; these must return, not panic.
	_, _ = RenderSQL("SELECT 1 {% raw %}{% endraw %}", nil)
	_, _ = Render("x {% raw %}{% endraw %}", nil)
}
