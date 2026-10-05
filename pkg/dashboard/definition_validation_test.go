package dashboard

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestValidateDefinition(t *testing.T) {
	text := []byte(`{"name":"Notes","rows":[{"widgets":[{"name":"Context","type":"text","content":"x"}]}]}`)
	semantic := []byte(`
name: Sales
model: s
models: {s: sales}
notes:
  - id: rollout
    dimensions: [{name: region, type: select}]
queries:
  totals: {dimensions: [{name: region}], metrics: [revenue]}
rows:
  - widgets:
      - {name: Revenue, type: chart, chart: bar, query: totals, notes: [rollout]}
`)
	sales := []byte(`{"name":"sales","source":{"table":"sales"},"dimensions":[{"name":"region","type":"string"}],"metrics":[{"name":"revenue","expression":"SUM(amount)"}]}`)
	for _, tt := range []struct {
		name   string
		data   []byte
		models map[string][]byte
		want   []string
	}{
		{name: "no models", data: text},
		{name: "semantic aliases and notes", data: semantic, models: map[string][]byte{"sales": sales}},
		{
			name: "missing referenced model", data: semantic,
			want: []string{`semantic model "sales" not found`},
		},
		{
			name: "invalid supplied models in stable order", data: text,
			models: map[string][]byte{"z": []byte(`{"name":"z"}`), "a": []byte(`{"name":"a"}`)},
			want:   []string{`semantic model "a": missing property 'source'`, `semantic model "z": missing property 'source'`},
		},
		{
			name: "name mismatch", data: text,
			models: map[string][]byte{"sales": []byte(`{"name":"orders","source":{"table":"orders"}}`)},
			want:   []string{`semantic model "sales": name must match its key, got "orders"`},
		},
		{
			name: "engine checks beyond schema", data: text,
			models: map[string][]byte{"sales": []byte(`{"name":"sales","source":{"table":"sales"},"dimensions":[{"name":"region","type":"string"},{"name":"region","type":"string"}]}`)},
			want:   []string{`semantic model "sales": duplicate name: region`},
		},
		{
			name: "full rules with no models", data: []byte(`{"name":"Sales","rows":[{"widgets":[{"name":"Revenue","type":"metric"}]}]}`),
			want: []string{`row 1, widget 1 ("Revenue"): value is required for metric widgets`},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDefinition(tt.data, tt.models)
			if tt.want == nil {
				assertNoErr(t, err)
				return
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
			for _, problem := range tt.want {
				if !strings.Contains(strings.Join(validationErr.Errors, "\n"), problem) {
					t.Errorf("missing %q in %v", problem, validationErr.Errors)
				}
			}
			if tt.name == "invalid supplied models in stable order" && !reflect.DeepEqual(validationErr.Errors, tt.want) {
				t.Fatalf("got %v, want %v", validationErr.Errors, tt.want)
			}
		})
	}
}

func TestValidateDefinition_SchemaFirst(t *testing.T) {
	err := ValidateDefinition([]byte(`{"name":"Broken"}`), map[string][]byte{"sales": []byte(`{"name":"sales"}`)})
	if err == nil || !strings.Contains(err.Error(), "rows") || strings.Contains(err.Error(), "semantic model") {
		t.Fatalf("expected dashboard schema failure first, got %v", err)
	}
}

func TestValidateDefinition_JoinDependencies(t *testing.T) {
	data := []byte(`
name: Sales
model: sales
queries:
  totals: {dimensions: [{name: customers.region}], metrics: [revenue]}
rows:
  - widgets:
      - {name: Revenue, type: chart, chart: bar, query: totals}
`)
	models := map[string][]byte{
		"sales": []byte(`
name: sales
source: {table: sales}
metrics: [{name: revenue, expression: "SUM(amount)"}]
joins: [{name: customers, relationship: many_to_one, foreign_key: customer_id}]
`),
		"customers": []byte(`
name: customers
source: {table: customers}
primary_key: id
dimensions: [{name: region, type: string}]
`),
	}
	assertNoErr(t, ValidateDefinition(data, models))
	delete(models, "customers")
	assertErr(t, ValidateDefinition(data, models))
	models["customers"] = []byte(`{"name":"customers","source":{"table":"customers"},"dimensions":[{"name":"region","type":"string"}]}`)
	err := ValidateDefinition(data, models)
	if err == nil || !strings.Contains(err.Error(), `requires target_key or primary_key`) {
		t.Fatalf("expected join target configuration failure, got %v", err)
	}
}
