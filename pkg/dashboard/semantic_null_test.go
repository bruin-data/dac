package dashboard

import (
	sem "github.com/bruin-data/bruin/semantic-engine"
	"testing"
)

func TestNullInclusiveFilterValidation(t *testing.T) {
	engine, err := sem.NewEngine(&sem.Model{Name: "orders", Source: sem.Source{Table: "orders"}, Dimensions: []sem.Dimension{{Name: "country", Type: "string"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []sem.Filter{{Dimension: "country", Operator: "equals", Value: []any{"US"}}, {Dimension: "country", Operator: "in", Value: []any{[]any{"US"}}}, {Dimension: "country} OR 1=1 -- {country", Operator: "equals", Value: "US"}} {
		if _, _, err := PrepareNullInclusiveQuery(sem.Query{Dimensions: []sem.DimensionRef{{Name: "country"}}, Filters: []sem.Filter{f}}, "", engine); err == nil {
			t.Fatalf("accepted invalid filter: %+v", f)
		}
	}
}
