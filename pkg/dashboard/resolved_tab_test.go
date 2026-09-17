package dashboard

import "testing"

// ResolvedTab must not mutate the parent: a tab overriding a pointer field
// (Horizontal) may not leak into the parent or sibling tabs.
func TestResolvedTab_NoParentMutation(t *testing.T) {
	horizTrue := true
	parent := Widget{
		Type:       WidgetTypeChart,
		Chart:      "bar",
		Horizontal: &horizTrue,
		Tabs: []Widget{
			{Name: "A", Horizontal: boolPtr(false)}, // overrides
			{Name: "B"},                             // inherits
		},
	}

	a := parent.ResolvedTab(0)
	if a.Horizontal == nil || *a.Horizontal != false {
		t.Fatalf("tab A should override horizontal=false, got %v", a.Horizontal)
	}
	if parent.Horizontal == nil || *parent.Horizontal != true {
		t.Fatalf("parent.Horizontal mutated: got %v, want true", parent.Horizontal)
	}

	b := parent.ResolvedTab(1)
	if b.Horizontal == nil || *b.Horizontal != true {
		t.Fatalf("tab B should inherit horizontal=true, got %v", b.Horizontal)
	}
	if b.Type != WidgetTypeChart || b.Chart != "bar" {
		t.Fatalf("tab B should inherit type/chart, got type=%q chart=%q", b.Type, b.Chart)
	}
	if len(b.Tabs) != 0 {
		t.Fatalf("resolved tab must not carry nested tabs, got %d", len(b.Tabs))
	}
}

// A tab's explicit false must override an inherited true (pointer bool fields).
func TestResolvedTab_FalseOverride(t *testing.T) {
	parent := Widget{
		Type:    WidgetTypeChart,
		Chart:   "bar",
		Stacked: boolPtr(true),
		Tabs: []Widget{
			{Name: "A", Stacked: boolPtr(false)}, // opts out
			{Name: "B"},                          // inherits true
		},
	}
	if a := parent.ResolvedTab(0); a.Stacked == nil || *a.Stacked != false {
		t.Fatalf("tab A should override stacked=false, got %v", a.Stacked)
	}
	if b := parent.ResolvedTab(1); b.Stacked == nil || *b.Stacked != true {
		t.Fatalf("tab B should inherit stacked=true, got %v", b.Stacked)
	}
}
