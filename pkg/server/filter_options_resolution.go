package server

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	tmpl "github.com/bruin-data/dac/pkg/template"
)

// ResolveFilterOptions resolves option values and display-only metrics. Static
// string lists retain their precedence over options queries.
func ResolveFilterOptions(ctx context.Context, backend query.Backend, d *dashboard.Dashboard, filters map[string]any) error {
	for i := range d.Filters {
		f := &d.Filters[i]
		if f.Type != "select" || f.Options == nil || len(f.Options.Values) > 0 || (f.Options.Query == "" && f.Options.Dimension == "") {
			continue
		}
		o := *f.Options
		f.Options = &o
		if err := resolveFilterOptions(ctx, backend, d, &o, filters); err != nil {
			return fmt.Errorf("resolving options for filter %q: %w", f.Name, err)
		}
	}
	return nil
}

func resolveFilterOptions(ctx context.Context, backend query.Backend, d *dashboard.Dashboard, o *dashboard.FilterOptions, filters map[string]any) error {
	connection := o.Connection
	if connection == "" {
		connection = d.Connection
	}
	metricField := ""
	if o.Metric != nil {
		metricField = o.Metric.Field
		if metricField == "" {
			metricField = o.Metric.Name
		}
	}
	var sql string
	var err error
	if o.Dimension != "" {
		job, resolveErr := d.ResolveFilterSemanticJob(o)
		if resolveErr != nil {
			return resolveErr
		}
		sql, connection, _, err = compileSemanticJob(job, filters, backend)
		if o.Metric != nil {
			for _, metric := range job.Model.Metrics {
				if metric.Name == metricField && metric.Format != nil {
					o.MetricFormat = &dashboard.SemanticFormat{
						Type: metric.Format.Type, Currency: metric.Format.Currency, Decimals: &metric.Format.Decimals,
					}
					break
				}
			}
		}
	} else {
		var dialect string
		dialect, err = query.ConnectionType(connection, backend)
		if err == nil {
			sql, err = tmpl.RenderSQL(o.Query, filters, dialect)
		}
	}
	if err != nil {
		return err
	}
	result, err := backend.Execute(ctx, connection, sql)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("query returned no result")
	}
	metricIndex := -1
	if metricField != "" {
		metricIndex = slices.IndexFunc(result.Columns, func(col query.ColumnInfo) bool { return col.Name == metricField })
		if metricIndex < 0 {
			// Warehouses such as Snowflake upper-case unquoted aliases.
			metricIndex = slices.IndexFunc(result.Columns, func(col query.ColumnInfo) bool { return strings.EqualFold(col.Name, metricField) })
		}
		if metricIndex < 0 {
			return fmt.Errorf("metric field %q not found in query results", metricField)
		}
	} else if o.Query != "" && o.Metric == nil && len(result.Columns) == 2 {
		metricIndex = 1
		metricField = result.Columns[1].Name
	}
	if metricIndex >= 0 {
		o.MetricValues = make(map[string]any)
	}
	o.Values = make([]string, 0, len(result.Rows))
	seen := make(map[string]bool)
	for _, row := range result.Rows {
		if len(row) == 0 || row[0] == nil {
			continue
		}
		value, err := optionValue(row[0])
		if err != nil {
			return fmt.Errorf("query must return scalar values in its first column")
		}
		if tmpl.ValidateSQLValue(value) != nil || seen[value] {
			continue
		}
		seen[value] = true
		o.Values = append(o.Values, value)
		if metricIndex >= 0 {
			var metric any
			if metricIndex < len(row) {
				metric = row[metricIndex]
			}
			if metric != nil {
				if _, err := optionValue(metric); err != nil {
					return fmt.Errorf("metric field %q must return scalar values", metricField)
				}
			}
			o.MetricValues[value] = metric
		}
	}
	return nil
}

func optionValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", fmt.Errorf("non-finite number")
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool, int, int64, uint64:
		return fmt.Sprint(v), nil
	default:
		return "", fmt.Errorf("not a scalar")
	}
}
