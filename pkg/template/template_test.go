package template

import (
	"testing"
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
			got, err := Render(tt.template, tt.filters)
			if tt.wantErr {
				assertErr(t, err)
				return
			}
			assertNoErr(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestRender_MissingFilter(t *testing.T) {
	t.Run("missing top-level key renders empty", func(t *testing.T) {
		got, err := Render("region={{ filters.region }}", map[string]any{})
		assertNoErr(t, err)
		assertEqual(t, got, "region=")
	})

	t.Run("missing nested key renders empty", func(t *testing.T) {
		got, err := Render("start={{ filters.date_range.start }}", map[string]any{"date_range": map[string]any{}})
		assertNoErr(t, err)
		assertEqual(t, got, "start=")
	})
}

func TestRender_BruinUserEmail(t *testing.T) {
	t.Run("resolves from BRUIN_USER_EMAIL", func(t *testing.T) {
		t.Setenv("BRUIN_USER_EMAIL", "viewer@example.com")
		got, err := Render("WHERE owner = '{{ bruin.user_email }}'", map[string]any{})
		assertNoErr(t, err)
		assertEqual(t, got, "WHERE owner = 'viewer@example.com'")
	})

	t.Run("renders empty when unset", func(t *testing.T) {
		t.Setenv("BRUIN_USER_EMAIL", "")
		got, err := Render("owner={{ bruin.user_email }}", map[string]any{})
		assertNoErr(t, err)
		assertEqual(t, got, "owner=")
	})
}

// Behavior that changed when gonja moved to Bruin's pinned commit, which
// follows Python Jinja2 more closely than v2.7.0 did.
func TestRender_JinjaParity(t *testing.T) {
	filters := map[string]any{"region": "EU", "empty": "", "list": []any{"a", "b"}}
	tests := []struct{ name, template, want string }{
		// or/and returned True or False before.
		{"or returns operand", "{{ filters.region or 'All' }}", "EU"},
		{"or falls back", "{{ filters.empty or 'All' }}", "All"},
		{"or missing", "{{ filters.missing or 'All' }}", "All"},
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
			got, err := Render(tt.template, filters)
			assertNoErr(t, err)
			assertEqual(t, got, tt.want)
		})
	}
}

func TestRender_InvalidTemplate(t *testing.T) {
	t.Run("malformed tag", func(t *testing.T) {
		_, err := Render("{{ bad syntax !@ }}", map[string]any{})
		assertErr(t, err)
	})

	t.Run("unclosed block", func(t *testing.T) {
		_, err := Render("{% if filters.x %} hello", map[string]any{})
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
