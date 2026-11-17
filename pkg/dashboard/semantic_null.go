package dashboard

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/google/uuid"
)

// PrepareNullInclusiveQuery preserves null rows for exclusions and supports explicit
// null members in inclusion and exclusion lists.
// Values are inserted only after compilation so braces in data cannot become
// semantic references. A single replacement pass also keeps values opaque.
func PrepareNullInclusiveQuery(q sem.Query, dialect string, engine *sem.Engine) (sem.Query, func(string) string, error) {
	// Validate structured dimensions before converting them to expression references.
	if _, err := engine.GenerateSQL(&q); err != nil {
		return q, nil, err
	}
	q.Filters = append([]sem.Filter(nil), q.Filters...)
	var replacements []string
	for i, f := range q.Filters {
		op := map[string]string{"equals": "=", "not_equals": "!=", "in": "IN", "not_in": "NOT IN"}[f.Operator]
		if f.Expression != "" || op == "" {
			continue
		}
		var values []any
		switch v := f.Value.(type) {
		case []any:
			values = v
		case []string:
			for _, item := range v {
				values = append(values, item)
			}
		default:
			values = []any{v}
		}
		list := f.Operator == "in" || f.Operator == "not_in"
		if len(values) == 0 || (!list && len(values) != 1) {
			return q, nil, fmt.Errorf("invalid structured filter value")
		}
		if !list {
			switch f.Value.(type) {
			case []any, []string:
				return q, nil, fmt.Errorf("comparison requires a scalar")
			}
		}
		var placeholders []string
		hasNull := false
		for _, value := range values {
			if value == nil && list {
				hasNull = true
				continue
			}
			literal, err := semanticNullLiteral(value, dialect)
			if err != nil {
				return q, nil, err
			}
			token := "'dac_null_" + strings.ReplaceAll(uuid.NewString(), "-", "") + "'"
			replacements = append(replacements, token, literal)
			placeholders = append(placeholders, token)
		}
		nullExpr := fmt.Sprintf("{%s} IS NULL", f.Dimension)
		negative := f.Operator == "not_in" || f.Operator == "not_equals"
		if negative && hasNull {
			nullExpr = fmt.Sprintf("{%s} IS NOT NULL", f.Dimension)
		}
		if len(placeholders) == 0 {
			q.Filters[i] = sem.Filter{Expression: nullExpr}
			continue
		}
		rhs := strings.Join(placeholders, ", ")
		if list {
			rhs = "(" + rhs + ")"
		}
		predicate := fmt.Sprintf("{%s} %s %s", f.Dimension, op, rhs)
		switch {
		case negative && hasNull:
			predicate = "(" + predicate + " AND " + nullExpr + ")"
		case negative || hasNull:
			predicate = "(" + predicate + " OR " + nullExpr + ")"
		}
		q.Filters[i] = sem.Filter{Expression: predicate}
	}
	return q, strings.NewReplacer(replacements...).Replace, nil
}

func semanticNullLiteral(value any, dialect string) (string, error) {
	switch v := value.(type) {
	case string:
		if dialect == "bigquery" || dialect == "google_cloud_platform" {
			return strconv.Quote(v), nil
		}
		return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float64:
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			return strconv.FormatFloat(v, 'g', -1, 64), nil
		}
	}
	return "", fmt.Errorf("structured filter requires non-null scalar values; use is_null to select only null rows")
}
