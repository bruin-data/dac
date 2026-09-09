package server

import (
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

// TestRenderSemanticQuery_MultiSelectFilter covers multi-select filters: a
// filled selection keeps its list shape, and an empty selection drops the
// filter, on both the value and expression authoring paths.
func TestRenderSemanticQuery_MultiSelectFilter(t *testing.T) {
	query := sem.Query{
		Filters: []sem.Filter{{
			Dimension: "platform",
			Operator:  "in",
			Value:     "{{ filters.platform }}",
		}},
	}

	t.Run("filled list stays a list", func(t *testing.T) {
		filters := map[string]any{"platform": []interface{}{"ios", "android"}}
		out, err := renderSemanticQuery(query, filters)
		if err != nil {
			t.Fatalf("renderSemanticQuery: %v", err)
		}
		if len(out.Filters) != 1 {
			t.Fatalf("expected 1 filter, got %d", len(out.Filters))
		}
		got, ok := out.Filters[0].Value.([]interface{})
		if !ok {
			t.Fatalf("expected []interface{} value, got %T (%v)", out.Filters[0].Value, out.Filters[0].Value)
		}
		if len(got) != 2 || got[0] != "ios" || got[1] != "android" {
			t.Fatalf("unexpected value: %v", got)
		}
	})

	t.Run("empty list drops the filter", func(t *testing.T) {
		filters := map[string]any{"platform": []interface{}{}}
		out, err := renderSemanticQuery(query, filters)
		if err != nil {
			t.Fatalf("renderSemanticQuery: %v", err)
		}
		if len(out.Filters) != 0 {
			t.Fatalf("expected empty multi-select to drop the filter, got %d filters", len(out.Filters))
		}
	})

	t.Run("empty prefix name keeps a different filter's constraint", func(t *testing.T) {
		q := sem.Query{Filters: []sem.Filter{{
			Dimension: "team_id",
			Operator:  "in",
			Value:     "{{ filters.team_id }}",
		}}}
		// team is empty, team_id is not: team must not drop the team_id filter.
		out, err := renderSemanticQuery(q, map[string]any{
			"team":    []interface{}{},
			"team_id": []interface{}{"7"},
		})
		if err != nil {
			t.Fatalf("renderSemanticQuery: %v", err)
		}
		if len(out.Filters) != 1 {
			t.Fatalf("expected team_id filter to survive, got %d filters", len(out.Filters))
		}
	})

	t.Run("empty selection drops an expression filter", func(t *testing.T) {
		exprQuery := sem.Query{
			Filters: []sem.Filter{{
				Expression: "platform IN ('{{ filters.platform | join(\"','\") }}')",
			}},
		}
		out, err := renderSemanticQuery(exprQuery, map[string]any{"platform": []interface{}{}})
		if err != nil {
			t.Fatalf("renderSemanticQuery: %v", err)
		}
		if len(out.Filters) != 0 {
			t.Fatalf("expected empty selection to drop the expression filter, got %d", len(out.Filters))
		}
	})

	t.Run("scalar still renders to a string", func(t *testing.T) {
		filters := map[string]any{"platform": "ios"}
		out, err := renderSemanticQuery(query, filters)
		if err != nil {
			t.Fatalf("renderSemanticQuery: %v", err)
		}
		if len(out.Filters) != 1 {
			t.Fatalf("expected 1 filter, got %d", len(out.Filters))
		}
		if out.Filters[0].Value != "ios" {
			t.Fatalf("expected scalar \"ios\", got %T (%v)", out.Filters[0].Value, out.Filters[0].Value)
		}
	})
}

// TestRenderTemplateValue_ListRefOnlyWhenBare checks that only a lone
// `{{ filters.x }}` keeps its list; anything with a pipe, dotted path, or extra
// text renders to a string instead.
func TestRenderTemplateValue_ListRefOnlyWhenBare(t *testing.T) {
	filters := map[string]any{"channel": []interface{}{"online", "retail"}}

	if got, _ := renderTemplateValue("{{ filters.channel }}", filters); !isList(got) {
		t.Errorf("bare reference should stay a list, got %T (%v)", got, got)
	}

	for _, in := range []string{
		"{{ filters.channel | join(\"','\") }}",
		"prefix {{ filters.channel }}",
	} {
		got, err := renderTemplateValue(in, filters)
		if err != nil {
			t.Fatalf("renderTemplateValue(%q): %v", in, err)
		}
		if _, ok := got.(string); !ok {
			t.Errorf("renderTemplateValue(%q) = %T, want rendered string", in, got)
		}
	}
}
