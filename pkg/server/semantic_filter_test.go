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
