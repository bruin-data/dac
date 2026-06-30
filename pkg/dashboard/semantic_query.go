package dashboard

import (
	"fmt"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

type SemanticJob struct {
	Model      *sem.Model
	ModelName  string
	Connection string
	Query      sem.Query
}

func (d *Dashboard) ResolveWidgetSemanticJob(w *Widget) (*SemanticJob, bool, error) {
	if w.QueryRef != "" {
		q, ok := d.Queries[w.QueryRef]
		if !ok || !q.IsSemantic() {
			return nil, false, nil
		}
		model, modelName, err := d.ResolveSemanticModel(q.Model)
		if err != nil {
			return nil, false, err
		}
		if model == nil {
			return nil, false, fmt.Errorf("named query %q does not specify a semantic model", w.QueryRef)
		}
		conn := q.Connection
		if conn == "" {
			conn = d.Connection
		}
		return &SemanticJob{
			Model:      model,
			ModelName:  modelName,
			Connection: conn,
			Query:      semanticQueryFromNamedQuery(q),
		}, true, nil
	}

	hasModelContext := w.Model != "" || d.Model != ""
	usesDirectSemantic := hasModelContext && (w.MetricRef != "" ||
		len(w.Dimensions) > 0 || w.Dimension != "" || w.Granularity != "" ||
		len(w.MetricRefs) > 0 || len(w.Filters) > 0 || len(w.Segments) > 0 || len(w.Sort) > 0)
	if !usesDirectSemantic {
		return nil, false, nil
	}

	model, modelName, err := d.ResolveSemanticModel(w.Model)
	if err != nil {
		return nil, false, err
	}
	if model == nil {
		return nil, false, fmt.Errorf("widget %q does not specify a semantic model", w.Name)
	}

	conn := w.Connection
	if conn == "" {
		conn = d.Connection
	}

	return &SemanticJob{
		Model:      model,
		ModelName:  modelName,
		Connection: conn,
		Query:      semanticQueryFromWidget(w),
	}, true, nil
}

func semanticQueryFromNamedQuery(q Query) sem.Query {
	return sem.Query{
		Dimensions: toSemanticDimensionRefs(q.Dimensions),
		Metrics:    append([]string(nil), q.Metrics...),
		Filters:    toSemanticFilters(q.Filters),
		Segments:   append([]string(nil), q.Segments...),
		Sort:       toSemanticSorts(q.Sort),
		Limit:      q.Limit,
	}
}

func semanticQueryFromWidget(w *Widget) sem.Query {
	dimensions := toSemanticDimensionRefs(w.Dimensions)
	if len(dimensions) == 0 && w.Dimension != "" {
		dimensions = []sem.DimensionRef{{
			Name:        w.Dimension,
			Granularity: w.Granularity,
		}}
	}

	metrics := append([]string(nil), w.MetricRefs...)
	if len(metrics) == 0 && w.MetricRef != "" {
		metrics = []string{w.MetricRef}
	}

	return sem.Query{
		Dimensions: dimensions,
		Metrics:    metrics,
		Filters:    toSemanticFilters(w.Filters),
		Segments:   append([]string(nil), w.Segments...),
		Sort:       toSemanticSorts(w.Sort),
		Limit:      w.Limit,
	}
}

func toSemanticDimensionRefs(refs []SemanticDimensionRef) []sem.DimensionRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]sem.DimensionRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, sem.DimensionRef{
			Name:        ref.Name,
			Granularity: ref.Granularity,
		})
	}
	return out
}

func toSemanticFilters(filters []SemanticQueryFilter) []sem.Filter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]sem.Filter, 0, len(filters))
	for _, filter := range filters {
		out = append(out, sem.Filter{
			Dimension:  filter.Dimension,
			Operator:   filter.Operator,
			Value:      filter.Value,
			Expression: filter.Expression,
		})
	}
	return out
}

func toSemanticSorts(sortSpecs []SemanticSort) []sem.SortSpec {
	if len(sortSpecs) == 0 {
		return nil
	}
	out := make([]sem.SortSpec, 0, len(sortSpecs))
	for _, sort := range sortSpecs {
		out = append(out, sem.SortSpec{
			Name:      sort.Name,
			Direction: sort.Direction,
		})
	}
	return out
}
