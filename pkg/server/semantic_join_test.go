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

	sql, conn, err := compileSemanticJob(job, nil)
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

	sql, _, err := compileSemanticJob(job, nil)
	if err != nil {
		t.Fatalf("compile single-model job: %v", err)
	}
	if strings.Contains(strings.ToUpper(sql), "JOIN") {
		t.Fatalf("did not expect a JOIN for a single-model query, got: %s", sql)
	}
}
