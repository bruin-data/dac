package dashboard

import (
	"math"
	"testing"
)

func TestValidateFilterValues(t *testing.T) {
	d := &Dashboard{Filters: []Filter{
		{Name: "date", Type: "date"}, {Name: "range", Type: "date-range"},
		{Name: "number", Type: "number"}, {Name: "text", Type: "text"},
		{Name: "select", Type: "select", Options: &FilterOptions{Values: []string{"US", "EU"}}},
		{Name: "dynamic", Type: "select", Options: &FilterOptions{Query: "SELECT region FROM regions"}},
		{Name: "multi", Type: "select", Multiple: true, Options: &FilterOptions{Values: []string{"US", "EU"}}},
	}}
	valid := []map[string]any{
		{}, {"date": "2024-02-29"}, {"range": map[string]any{"start": "2024-01-01", "end": "2024-02-29"}},
		{"number": 0}, {"number": -1.5}, {"text": "hello world"}, {"text": ""},
		{"select": "EU"}, {"dynamic": "custom value"},
		{"date": ""}, {"date": nil}, {"number": ""}, {"select": ""}, {"multi": []any{"EU", "US"}}, {"multi": []string{}},
	}
	for _, values := range valid {
		if err := d.ValidateFilterValues(values); err != nil {
			t.Errorf("%v: %v", values, err)
		}
	}
	invalid := []map[string]any{
		{"missing": "value"}, {"date": "2023-02-29"}, {"date": "2024-1-01"}, {"date": "TODAY-1"},
		{"range": "last_7_days"}, {"range": map[string]any{"start": "2024-01-01"}},
		{"number": "123"}, {"number": math.Inf(1)}, {"number": math.NaN()}, {"number": false},
		{"text": 12}, {"text": map[string]any{}}, {"select": "CA"}, {"dynamic": 12},
		{"multi": []any{"CA"}}, {"multi": []any{12}}, {"multi": "EU"},
	}
	for _, values := range invalid {
		if err := d.ValidateFilterValues(values); err == nil {
			t.Errorf("accepted %v", values)
		}
	}
}

func TestValidateResolvedDefaults(t *testing.T) {
	d := &Dashboard{Filters: []Filter{
		{Name: "date", Type: "date", Default: "TODAY-1"},
		{Name: "range", Type: "date-range", Default: "last_30_days"},
		{Name: "number", Type: "number", Default: 10},
	}}
	if err := d.ValidateFilterValues(d.DefaultFilters()); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeFilterValues(t *testing.T) {
	d := &Dashboard{Filters: []Filter{
		{Name: "date", Type: "date", Default: "TODAY-1"},
		{Name: "year", Type: "select", Default: 2024, Options: &FilterOptions{Values: []string{"2023", "2024"}}},
		{Name: "years", Type: "select", Multiple: true, Options: &FilterOptions{Values: []string{"2023", "2024"}}},
		{Name: "range", Type: "date-range", Default: "last_7_days"},
	}}
	// The frontend echoes YAML defaults back verbatim.
	values := d.NormalizeFilterValues(map[string]any{"date": "TODAY-1", "year": float64(2024), "years": []any{2023}, "range": "last_7_days"})
	if err := d.ValidateFilterValues(values); err != nil {
		t.Fatal(err)
	}
	if values["date"] != ResolveDateExpression("TODAY-1") || values["year"] != "2024" {
		t.Errorf("unexpected normalized values: %v", values)
	}
	if err := d.ValidateFilterValues(d.NormalizeFilterValues(d.DefaultFilters())); err != nil {
		t.Fatal(err)
	}
}
