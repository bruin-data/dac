package server

import (
	"strings"
	"testing"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
)

// TestCompileSemanticJob_CrossModelJoin verifies that a query referencing a
// dimension on a joined model compiles to SQL with a JOIN against that model.
func TestCompileSemanticJob_CrossModelJoin(t *testing.T) {
	customers := &sem.Model{
		Name:       "customers",
		Source:     sem.Source{Table: "public.customers"},
		PrimaryKey: "customer_id",
		Dimensions: []sem.Dimension{{Name: "country", Type: "string"}},
	}
	orders := &sem.Model{
		Name:       "orders",
		Source:     sem.Source{Table: "public.orders"},
		PrimaryKey: "order_id",
		Joins: []sem.Join{{
			Name:         "customers",
			Relationship: "many_to_one",
			ForeignKey:   "customer_id",
		}},
		Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}},
	}

	job := &dashboard.SemanticJob{
		Model:      orders,
		ModelName:  "orders",
		Connection: "warehouse",
		Models:     map[string]*sem.Model{"orders": orders, "customers": customers},
		Query: sem.Query{
			Dimensions: []sem.DimensionRef{{Name: "customers.country"}},
			Metrics:    []string{"revenue"},
		},
	}

	sql, conn, renames, err := compileSemanticJob(job, nil)
	if err != nil {
		t.Fatalf("compile cross-model job: %v", err)
	}
	if conn != "warehouse" {
		t.Fatalf("expected connection warehouse, got %q", conn)
	}
	if !strings.Contains(strings.ToUpper(sql), "JOIN") {
		t.Fatalf("expected a JOIN in cross-model SQL, got: %s", sql)
	}
	if !strings.Contains(sql, "customers") || !strings.Contains(sql, "country") {
		t.Fatalf("expected joined customers.country in SQL, got: %s", sql)
	}
	// The sanitized output column must map back to the dashboard-facing name.
	if got := renames["customers_country"]; got != "customers.country" {
		t.Fatalf("expected rename customers_country -> customers.country, got %q (%v)", got, renames)
	}
}

// TestCompileSemanticJob_SingleModelNoJoin verifies a single-model query stays
// join-free even though the engine is always built with the model set.
func TestCompileSemanticJob_SingleModelNoJoin(t *testing.T) {
	orders := &sem.Model{
		Name:    "orders",
		Source:  sem.Source{Table: "public.orders"},
		Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}},
	}

	job := &dashboard.SemanticJob{
		Model:      orders,
		ModelName:  "orders",
		Connection: "warehouse",
		Models:     map[string]*sem.Model{"orders": orders},
		Query:      sem.Query{Metrics: []string{"revenue"}},
	}

	sql, _, renames, err := compileSemanticJob(job, nil)
	if err != nil {
		t.Fatalf("compile single-model job: %v", err)
	}
	if strings.Contains(strings.ToUpper(sql), "JOIN") {
		t.Fatalf("did not expect a JOIN for a single-model query, got: %s", sql)
	}
	if len(renames) != 0 {
		t.Fatalf("did not expect column renames for a single-model query, got: %v", renames)
	}
}

// TestCompileSemanticJob_UnqualifiedJoinedGranularity verifies an unqualified
// time dimension from a joined model maps its granularity alias back too.
func TestCompileSemanticJob_UnqualifiedJoinedGranularity(t *testing.T) {
	customers := &sem.Model{
		Name:       "customers",
		Source:     sem.Source{Table: "public.customers"},
		PrimaryKey: "customer_id",
		Dimensions: []sem.Dimension{{Name: "signup_date", Type: "time", Granularities: map[string]string{"month": "date_trunc('month', signup_date)"}}},
	}
	orders := &sem.Model{
		Name:    "orders",
		Source:  sem.Source{Table: "public.orders"},
		Joins:   []sem.Join{{Name: "customers", Relationship: "many_to_one", ForeignKey: "customer_id"}},
		Metrics: []sem.Metric{{Name: "revenue", Expression: "sum(amount)"}},
	}
	job := &dashboard.SemanticJob{
		Model:     orders,
		ModelName: "orders",
		Models:    map[string]*sem.Model{"orders": orders, "customers": customers},
		Query: sem.Query{
			Dimensions: []sem.DimensionRef{{Name: "signup_date", Granularity: "month"}},
			Metrics:    []string{"revenue"},
		},
	}

	sql, _, renames, err := compileSemanticJob(job, nil)
	if err != nil {
		t.Fatalf("compile job: %v", err)
	}
	if !strings.Contains(sql, "signup_date_month") {
		t.Fatalf("expected engine alias signup_date_month, got: %s", sql)
	}
	if got := renames["signup_date_month"]; got != "signup_date" {
		t.Fatalf("expected rename signup_date_month -> signup_date, got %q (%v)", got, renames)
	}
}

// Only the models a job can reach are rendered: a model this widget does not
// use may reference filters this dashboard does not declare.
func TestCompileSemanticJob_RendersOnlyJoinedModels(t *testing.T) {
	other := &sem.Model{
		Name:     "other",
		Source:   sem.Source{Table: "other"},
		Segments: []sem.Segment{{Name: "mine", Filter: "{% if filters.region %}region = '{{ filters.region }}'{% else %}1=1{% endif %}"}},
	}
	customers := &sem.Model{
		Name:       "customers",
		Source:     sem.Source{Table: "customers"},
		PrimaryKey: "customer_id",
		Dimensions: []sem.Dimension{{Name: "country", Type: "string", Expression: "{% if filters.lower %}lower(country){% else %}country{% endif %}"}},
	}
	orders := &sem.Model{
		Name:       "orders",
		Source:     sem.Source{Table: "orders"},
		PrimaryKey: "order_id",
		Joins:      []sem.Join{{Name: "customers", Relationship: "many_to_one", ForeignKey: "customer_id"}},
		Metrics:    []sem.Metric{{Name: "n", Expression: "count(*)"}},
	}
	job := &dashboard.SemanticJob{
		Model:     orders,
		ModelName: "orders",
		Models:    map[string]*sem.Model{"orders": orders, "customers": customers, "other": other},
		Query:     sem.Query{Dimensions: []sem.DimensionRef{{Name: "customers.country"}}, Metrics: []string{"n"}},
	}
	// "region" is not declared here; "lower" is, but unset.
	if _, _, _, err := compileSemanticJob(job, map[string]any{"lower": nil}); err != nil {
		t.Fatalf("an unused model failed the job: %v", err)
	}
	// The joined model is still rendered, strictly.
	if sql, _, _, err := compileSemanticJob(job, map[string]any{}); err == nil {
		t.Fatalf("joined model rendered an undefined name: %s", sql)
	}
}

// Joins are followed by target model (which may differ from the join name)
// and transitively, as the engine walks them.
func TestJoinedModels(t *testing.T) {
	regions := &sem.Model{Name: "regions"}
	customers := &sem.Model{Name: "customers", Joins: []sem.Join{{Name: "region", Model: "regions"}}}
	orders := &sem.Model{Name: "orders", Joins: []sem.Join{{Name: "buyer", Model: " customers "}}}
	other := &sem.Model{Name: "other", Joins: []sem.Join{{Name: "orders"}}}
	job := &dashboard.SemanticJob{
		Model:     orders,
		ModelName: "orders",
		Models:    map[string]*sem.Model{"orders": orders, "customers": customers, "regions": regions, "other": other},
	}
	got := joinedModels(job)
	if len(got) != 3 || got["orders"] != orders || got["customers"] != customers || got["regions"] != regions {
		t.Fatalf("got %v", got)
	}
}
