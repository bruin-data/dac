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
