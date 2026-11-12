package template

import (
	"testing"

	"github.com/nikolalohinski/gonja/v2"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		template string
		filters  map[string]any
		want     string
		wantErr  bool
	}{
		{
			name:     "simple variable substitution",
			template: "SELECT * FROM events WHERE region = '{{ filters.region }}'",
			filters:  map[string]any{"region": "US"},
			want:     "SELECT * FROM events WHERE region = 'US'",
		},
		{
			name:     "nested map access",
			template: "WHERE date >= '{{ filters.date_range.start }}'",
			filters:  map[string]any{"date_range": map[string]any{"start": "2025-01-01"}},
			want:     "WHERE date >= '2025-01-01'",
		},
		{
			name:     "conditional block true",
			template: "SELECT * FROM events WHERE 1=1 {% if filters.region != 'All' %}AND region = '{{ filters.region }}'{% endif %}",
			filters:  map[string]any{"region": "EU"},
			want:     "SELECT * FROM events WHERE 1=1 AND region = 'EU'",
		},
		{
			name:     "conditional block false",
			template: "SELECT * FROM events WHERE 1=1 {% if filters.region != 'All' %}AND region = '{{ filters.region }}'{% endif %}",
			filters:  map[string]any{"region": "All"},
			want:     "SELECT * FROM events WHERE 1=1 ",
		},
		{
			name:     "no-op passthrough plain string",
			template: "SELECT * FROM events WHERE region = 'US'",
			filters:  map[string]any{"region": "EU"},
			want:     "SELECT * FROM events WHERE region = 'US'",
		},
		{
			name:     "no-op passthrough with curly but not template",
			template: `{"key": "value"}`,
			filters:  map[string]any{},
			want:     `{"key": "value"}`,
		},
		{
			name:     "empty template string",
			template: "",
			filters:  map[string]any{"region": "US"},
			want:     "",
		},
		{
			name:     "multiple substitutions",
			template: "SELECT * FROM {{ filters.table }} WHERE region = '{{ filters.region }}' AND status = '{{ filters.status }}'",
			filters:  map[string]any{"table": "events", "region": "US", "status": "active"},
			want:     "SELECT * FROM events WHERE region = 'US' AND status = 'active'",
		},
		{
			name:     "nil filters map",
			template: "SELECT * FROM events",
			filters:  nil,
			want:     "SELECT * FROM events",
		},
		{
			name:     "nil filters with no markers",
			template: "plain text",
			filters:  nil,
			want:     "plain text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.template, tt.filters, "")
			if tt.wantErr {
				assertErr(t, err)
				return
			}
			assertNoErr(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

// Templates are strict like bruin's: a name that is not defined is an error,
// while a declared filter left unset is nil and renders as empty.
func TestRender_StrictUndefined(t *testing.T) {
	for _, tmpl := range []string{
		"region={{ filters.regoin }}",
		"start={{ filters.date_range.strat }}",
		"{% if filters.regoin %}x{% endif %}",
		"{{ filters.regoin or 'All' }}",
		"{{ regoin }}",
	} {
		if got, err := Render(tmpl, map[string]any{"region": nil, "date_range": map[string]any{"start": "2024-01-01"}}, ""); err == nil {
			t.Errorf("%q: undefined name rendered %q", tmpl, got)
		}
	}
	unset := map[string]any{"region": nil}
	for tmpl, want := range map[string]string{
		"region={{ filters.region }}":                       "region=",
		"{% if filters.region %}x{% else %}none{% endif %}": "none",
		"{{ filters.region or 'All' }}":                     "All",
		"{{ filters.region | default('All') }}":             "All",
		"{{ filters.other is defined }}":                    "False",
		"{{ filters.other | default('All') }}":              "All",
	} {
		got, err := Render(tmpl, unset, "")
		assertNoErr(t, err)
		assertEqual(t, got, want)
	}
}

// dac renders the same standalone and inside bruin, which changes gonja's
// shared default config.
func TestRender_IgnoresGonjaDefaultConfig(t *testing.T) {
	saved := gonja.DefaultConfig.StrictUndefined
	t.Cleanup(func() { gonja.DefaultConfig.StrictUndefined = saved })
	filters := map[string]any{"region": nil, "x": "EU"}
	for _, strict := range []bool{false, true} {
		gonja.DefaultConfig.StrictUndefined = strict
		got, err := Render("{{ filters.region or 'All' }}", filters, "")
		assertNoErr(t, err)
		assertEqual(t, got, "All")
		if _, err := Render("{{ filters.regoin }}", filters, ""); err == nil {
			t.Errorf("StrictUndefined=%v: Render accepted an undefined name", strict)
		}
		sql, err := RenderSQL("SELECT 1 {% if filters.region %}WHERE r = '{{ filters.region }}'{% endif %} AND x = '{{ filters.x }}'", filters)
		assertNoErr(t, err)
		assertEqual(t, sql, "SELECT 1  AND x = 'EU'")
		if _, err := RenderSQL("SELECT '{{ filters.regoin }}'", filters); err == nil {
			t.Errorf("StrictUndefined=%v: RenderSQL accepted an undefined name", strict)
		}
	}
}

func TestRender_BruinUserEmail(t *testing.T) {
	t.Run("resolves from BRUIN_USER_EMAIL", func(t *testing.T) {
		t.Setenv("BRUIN_USER_EMAIL", "viewer@example.com")
		got, err := Render("WHERE owner = '{{ bruin.user_email }}'", map[string]any{}, "")
		assertNoErr(t, err)
		assertEqual(t, got, "WHERE owner = 'viewer@example.com'")
	})

	t.Run("renders empty when unset", func(t *testing.T) {
		t.Setenv("BRUIN_USER_EMAIL", "")
		got, err := Render("owner={{ bruin.user_email }}", map[string]any{}, "")
		assertNoErr(t, err)
		assertEqual(t, got, "owner=")
	})
}

// Behavior that changed when gonja moved to Bruin's pinned commit, which
// follows Python Jinja2 more closely than v2.7.0 did.
func TestRender_JinjaParity(t *testing.T) {
	filters := map[string]any{"region": "EU", "empty": "", "unset": nil, "list": []any{"a", "b"}}
	tests := []struct{ name, template, want string }{
		// or/and returned True or False before.
		{"or returns operand", "{{ filters.region or 'All' }}", "EU"},
		{"or falls back", "{{ filters.empty or 'All' }}", "All"},
		{"or unset", "{{ filters.unset or 'All' }}", "All"},
		{"and returns operand", "{{ filters.region and 'set' }}", "set"},
		{"and short-circuits", "{{ filters.empty and 'set' }}", ""},
		// Lowercase Jinja2 loop attributes were undefined or not callable.
		{"loop length", "{% for x in filters.list %}{{ loop.length }}{% endfor %}", "22"},
		{"loop previtem nextitem", "{% for x in filters.list %}{{ loop.previtem }}|{{ loop.nextitem }};{% endfor %}", "|b;a|;"},
		{"loop cycle", "{% for x in filters.list %}{{ loop.cycle('x', 'y') }}{% endfor %}", "xy"},
		{"loop changed", "{% for x in ['a', 'a', 'b'] %}{{ loop.changed(x) }}{% endfor %}", "TrueFalseTrue"},
		{"loop depth", "{% for x in filters.list %}{{ loop.depth }}{% endfor %}", "11"},
		{"capitalized loop names", "{% for x in filters.list %}{{ loop.Cycle('x', 'y') }}{{ loop.PrevItem }}{% endfor %}", "xya"},
		// Line endings render as "\n" and one trailing newline is dropped.
		{"crlf", "a\r\nb {{ filters.region }}\rc", "a\nb EU\nc"},
		{"trailing newline", "{{ filters.region }}\n", "EU"},
		{"numeric literals", "{{ 1_000 }} {{ 0x10 }} {{ 1e3 }}", "1000 16 1000.0"},
		{"slice past end", "{{ filters.region[5:] }}{{ filters.list[1:10] }}", "['b']"},
		{"truncate leeway", "{{ 'abcdef' | truncate(4) }}", "abcdef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Render(tt.template, filters, "")
			assertNoErr(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestRender_InvalidTemplate(t *testing.T) {
	t.Run("malformed tag", func(t *testing.T) {
		_, err := Render("{{ bad syntax !@ }}", map[string]any{}, "")
		assertErr(t, err)
	})

	t.Run("unclosed block", func(t *testing.T) {
		_, err := Render("{% if filters.x %} hello", map[string]any{}, "")
		assertErr(t, err)
	})
}

// --- helpers ---

func assertNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertErr(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func assertEqual(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
