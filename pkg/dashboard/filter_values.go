package dashboard

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"time"
)

// NormalizeFilterValues returns a copy of values with client-sent defaults
// put in canonical form: date expressions such as TODAY-1 are resolved and
// scalar select values (YAML `default: 2024`) become strings. Unknown keys and
// other shapes are left for ValidateFilterValues to reject.
func (d *Dashboard) NormalizeFilterValues(values map[string]any) map[string]any {
	definitions := make(map[string]Filter, len(d.Filters))
	for _, f := range d.Filters {
		definitions[f.Name] = f
	}
	out := make(map[string]any, len(values))
	for name, value := range values {
		if f, ok := definitions[name]; ok {
			value = normalizeFilterValue(f, value)
		}
		out[name] = value
	}
	return out
}

func normalizeFilterValue(f Filter, value any) any {
	switch f.Type {
	case "date":
		if t, ok := value.(time.Time); ok {
			return t.Format("2006-01-02")
		}
		if expr, ok := value.(string); ok {
			if resolved := ResolveDateExpression(expr); resolved != "" {
				return resolved
			}
		}
	case "select":
		if list, ok := value.([]any); ok && f.Multiple {
			out := make([]any, len(list))
			for i, item := range list {
				out[i] = selectString(item)
			}
			return out
		}
		return selectString(value)
	}
	return value
}

func selectString(value any) any {
	switch value.(type) {
	case bool, int, int64, uint64, float64:
		return fmt.Sprint(value)
	}
	return value
}

// ValidateFilterValues checks the merged defaults and request values before
// rendering any query. Unknown keys and unexpected JSON shapes fail closed.
func (d *Dashboard) ValidateFilterValues(values map[string]any) error {
	definitions := make(map[string]Filter, len(d.Filters))
	for _, f := range d.Filters {
		definitions[f.Name] = f
	}
	for name, value := range values {
		f, ok := definitions[name]
		if !ok {
			return fmt.Errorf("unknown filter %q", name)
		}
		if err := validateFilterValue(f, value); err != nil {
			return fmt.Errorf("filter %q: %w", name, err)
		}
	}
	return nil
}

func validateFilterValue(f Filter, value any) error {
	// A cleared input (e.g. an emptied number box) is an empty value, not an
	// error; it carries nothing into SQL and templates can test for it.
	if (value == nil || value == "") && f.Type != "date-range" && !f.Multiple {
		return nil
	}
	switch f.Type {
	case "date":
		if !validFilterDate(value) {
			return fmt.Errorf("expected a valid YYYY-MM-DD date")
		}
	case "date-range":
		rangeValue, ok := value.(map[string]any)
		if !ok || len(rangeValue) != 2 || !validFilterDate(rangeValue["start"]) || !validFilterDate(rangeValue["end"]) {
			return fmt.Errorf("expected start and end dates in YYYY-MM-DD format")
		}
		if rangeValue["start"].(string) > rangeValue["end"].(string) {
			return fmt.Errorf("start date must not be after end date")
		}
	case "number":
		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		case reflect.Float32, reflect.Float64:
			if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
				return fmt.Errorf("expected a finite number")
			}
		default:
			return fmt.Errorf("expected a finite number")
		}
	case "text":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected text")
		}
	case "select":
		check := func(value any) error {
			s, ok := value.(string)
			if !ok {
				return fmt.Errorf("expected a string selection")
			}
			if f.Options != nil && len(f.Options.Values) > 0 && !slices.Contains(f.Options.Values, s) {
				return fmt.Errorf("value is not an allowed option")
			}
			return nil
		}
		if !f.Multiple {
			return check(value)
		}
		switch values := value.(type) {
		case []any:
			for _, v := range values {
				if err := check(v); err != nil {
					return err
				}
			}
		case []string:
			for _, v := range values {
				if err := check(v); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("expected an array of selections")
		}
	default:
		return fmt.Errorf("unsupported filter type %q", f.Type)
	}
	return nil
}

func validFilterDate(value any) bool {
	s, ok := value.(string)
	if !ok || len(s) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
