package server

import (
	"fmt"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"github.com/bruin-data/dac/pkg/dashboard"
	tmpl "github.com/bruin-data/dac/pkg/template"
)

func compileSemanticJob(job *dashboard.SemanticJob, filters map[string]any) (string, string, map[string]string, error) {
	model, err := renderSemanticModel(job.Model, filters)
	if err != nil {
		return "", "", nil, err
	}

	// Render every model in the project so queries that reference dimensions on
	// joined models compile against rendered (templated) expressions too.
	models := make(map[string]*sem.Model, len(job.Models))
	for name, m := range job.Models {
		rendered, err := renderSemanticModel(m, filters)
		if err != nil {
			return "", "", nil, fmt.Errorf("rendering semantic model %q: %w", name, err)
		}
		models[name] = rendered
	}
	// Keep the primary model identical to its entry in the rendered set so the
	// engine resolves joins against consistent model pointers.
	if job.ModelName != "" {
		if rendered, ok := models[job.ModelName]; ok {
			model = rendered
		}
	}

	query, err := renderSemanticQuery(job.Query, filters)
	if err != nil {
		return "", "", nil, err
	}

	engine, err := sem.NewEngineWithModels(model, models)
	if err != nil {
		return "", "", nil, err
	}
	sql, err := engine.GenerateSQL(&query)
	if err != nil {
		return "", "", nil, err
	}

	return sql, job.Connection, semanticColumnRenames(query), nil
}

// semanticColumnRenames maps the column names the engine emits back to the
// dimension names the dashboard references. The engine sanitizes qualified
// (joined) dimension names — e.g. "customers.country" is selected as
// "customers_country" — so without this remap the widget would look up a
// column that does not exist in the result. Non-qualified dimensions keep
// their name and need no entry.
func semanticColumnRenames(query sem.Query) map[string]string {
	renames := map[string]string{}
	for _, dim := range query.Dimensions {
		alias := semanticOutputAlias(dim)
		if alias != dim.Name {
			renames[alias] = dim.Name
		}
	}
	return renames
}

// semanticOutputAlias mirrors the engine's output-column naming for a
// dimension reference: only qualified names (containing ".") are sanitized,
// replacing the dot (and granularity separator) with an underscore.
func semanticOutputAlias(ref sem.DimensionRef) string {
	if !strings.Contains(ref.Name, ".") {
		return ref.Name
	}
	alias := strings.ReplaceAll(ref.Name, ".", "_")
	if ref.Granularity != "" {
		alias += "_" + strings.ReplaceAll(ref.Granularity, ".", "_")
	}
	return alias
}

func renderSemanticModel(model *sem.Model, filters map[string]any) (*sem.Model, error) {
	if model == nil {
		return nil, fmt.Errorf("semantic model is required")
	}

	clone := *model
	clone.Source = model.Source
	var err error
	clone.Source.Table, err = renderTemplateString(model.Source.Table, filters)
	if err != nil {
		return nil, fmt.Errorf("rendering semantic source table: %w", err)
	}

	clone.Dimensions = make([]sem.Dimension, len(model.Dimensions))
	for i, dim := range model.Dimensions {
		clonedDim := dim
		clonedDim.Expression, err = renderTemplateString(dim.Expression, filters)
		if err != nil {
			return nil, fmt.Errorf("rendering dimension %q expression: %w", dim.Name, err)
		}
		if len(dim.Granularities) > 0 {
			clonedDim.Granularities = make(map[string]string, len(dim.Granularities))
			for key, value := range dim.Granularities {
				clonedDim.Granularities[key], err = renderTemplateString(value, filters)
				if err != nil {
					return nil, fmt.Errorf("rendering dimension %q granularity %q: %w", dim.Name, key, err)
				}
			}
		}
		clone.Dimensions[i] = clonedDim
	}

	clone.Metrics = make([]sem.Metric, len(model.Metrics))
	for i, metric := range model.Metrics {
		clonedMetric := metric
		clonedMetric.Expression, err = renderTemplateString(metric.Expression, filters)
		if err != nil {
			return nil, fmt.Errorf("rendering metric %q expression: %w", metric.Name, err)
		}
		clonedMetric.Filter, err = renderTemplateString(metric.Filter, filters)
		if err != nil {
			return nil, fmt.Errorf("rendering metric %q filter: %w", metric.Name, err)
		}
		clone.Metrics[i] = clonedMetric
	}

	clone.Segments = make([]sem.Segment, len(model.Segments))
	for i, segment := range model.Segments {
		clonedSegment := segment
		clonedSegment.Filter, err = renderTemplateString(segment.Filter, filters)
		if err != nil {
			return nil, fmt.Errorf("rendering segment %q filter: %w", segment.Name, err)
		}
		clone.Segments[i] = clonedSegment
	}

	return &clone, nil
}

func renderSemanticQuery(query sem.Query, filters map[string]any) (sem.Query, error) {
	rendered := query
	if len(query.Dimensions) > 0 {
		rendered.Dimensions = append([]sem.DimensionRef(nil), query.Dimensions...)
	}
	if len(query.Metrics) > 0 {
		rendered.Metrics = append([]string(nil), query.Metrics...)
	}
	if len(query.Segments) > 0 {
		rendered.Segments = append([]string(nil), query.Segments...)
	}
	if len(query.Sort) > 0 {
		rendered.Sort = append([]sem.SortSpec(nil), query.Sort...)
	}

	if len(query.Filters) == 0 {
		return rendered, nil
	}

	rendered.Filters = make([]sem.Filter, len(query.Filters))
	for i, filter := range query.Filters {
		rendered.Filters[i] = filter
		var err error
		rendered.Filters[i].Expression, err = renderTemplateString(filter.Expression, filters)
		if err != nil {
			return sem.Query{}, fmt.Errorf("rendering filter expression: %w", err)
		}
		rendered.Filters[i].Value, err = renderTemplateValue(filter.Value, filters)
		if err != nil {
			return sem.Query{}, fmt.Errorf("rendering filter value: %w", err)
		}
	}

	return rendered, nil
}

func renderTemplateString(value string, filters map[string]any) (string, error) {
	if value == "" {
		return value, nil
	}
	// Render even with no filters: SQL may reference the `bruin` namespace
	// (e.g. {{ bruin.user_email }}). Render() short-circuits plain strings.
	return tmpl.Render(value, filters)
}

func renderTemplateValue(value any, filters map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderTemplateString(typed, filters)
	case []string:
		out := make([]string, len(typed))
		for i, item := range typed {
			rendered, err := renderTemplateString(item, filters)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, item := range typed {
			rendered, err := renderTemplateValue(item, filters)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			rendered, err := renderTemplateValue(item, filters)
			if err != nil {
				return nil, err
			}
			out[key] = rendered
		}
		return out, nil
	default:
		return value, nil
	}
}
