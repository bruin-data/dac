package dashboard

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

type SemanticJob struct {
	Model      *sem.Model
	ModelName  string
	Connection string
	Query      sem.Query
	// Models is the full set of semantic models in the project, keyed by name.
	// It is passed to the engine so queries can reference dimensions on joined
	// models (e.g. "customers.country").
	Models map[string]*sem.Model
	// Breakdowns maps breakdown filter names to their options.
	Breakdowns map[string][]BreakdownOption
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
			Models:     d.semanticModels,
			Breakdowns: d.breakdowns(),
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
		Models:     d.semanticModels,
		Breakdowns: d.breakdowns(),
	}, true, nil
}

func (d *Dashboard) breakdowns() map[string][]BreakdownOption {
	out := map[string][]BreakdownOption{}
	for _, f := range d.Filters {
		if f.Type == "breakdown" && f.Options != nil {
			out[f.Name] = f.Options.Breakdown
		}
	}
	return out
}

var breakdownRef = regexp.MustCompile(`^\{\{\s*filters\.(\w+)\s*\}\}$`)

// breakdownRefName returns the breakdown filter a dimension name like
// "{{ filters.geo }}" refers to.
func (j *SemanticJob) breakdownRefName(name string) (string, bool) {
	m := breakdownRef.FindStringSubmatch(strings.TrimSpace(name))
	if m == nil {
		return "", false
	}
	_, ok := j.Breakdowns[m[1]]
	return m[1], ok
}

// expandBreakdowns replaces each breakdown reference in dims with the selected
// option's dimensions, keeping list order and skipping duplicates. An empty
// selection drops the reference.
func (j *SemanticJob) expandBreakdowns(dims []sem.DimensionRef, values map[string]any) ([]sem.DimensionRef, error) {
	var out []sem.DimensionRef
	add := func(ref sem.DimensionRef) {
		if !slices.Contains(out, ref) {
			out = append(out, ref)
		}
	}
	for _, dim := range dims {
		name, ok := j.breakdownRefName(dim.Name)
		if !ok {
			add(dim)
			continue
		}
		label, _ := values[name].(string)
		if label == "" {
			continue
		}
		i := slices.IndexFunc(j.Breakdowns[name], func(o BreakdownOption) bool { return o.Label == label })
		if i < 0 {
			return nil, fmt.Errorf("filter %q: %q is not a breakdown option", name, label)
		}
		for _, d := range j.Breakdowns[name][i].Dimensions {
			ref, err := sem.ParseDimensionRef(d)
			if err != nil {
				return nil, err
			}
			add(ref)
		}
	}
	return out, nil
}

// SeriesQuery returns a model sparkline's x dimension and y metric; ok is false otherwise.
func (c TableColumn) SeriesQuery(model *sem.Model) (by sem.DimensionRef, metric string, ok bool) {
	xs, ys := c.X.FieldList(), c.Y.FieldList()
	if c.Type != "sparkline" || model == nil || len(xs) != 1 || len(ys) != 1 ||
		!slices.ContainsFunc(model.Metrics, func(m sem.Metric) bool { return m.Name == ys[0] }) ||
		slices.ContainsFunc(model.Metrics, func(m sem.Metric) bool { return m.Name == c.Name }) ||
		slices.ContainsFunc(model.Dimensions, func(d sem.Dimension) bool { return d.Name == c.Name }) {
		return sem.DimensionRef{}, "", false
	}
	name, granularity, _ := strings.Cut(xs[0], ":")
	return sem.DimensionRef{Name: strings.TrimSpace(name), Granularity: strings.TrimSpace(granularity)}, ys[0], true
}

func (c TableColumn) SeriesLimit() int {
	if c.Limit > 0 {
		return c.Limit
	}
	return MaxSparklinePoints
}

const MaxSparklinePoints = 100

// ExpandQuery expands breakdown references in the query's dimensions and sort.
func (j *SemanticJob) ExpandQuery(q sem.Query, values map[string]any) (sem.Query, error) {
	var err error
	if q.Dimensions, err = j.expandBreakdowns(q.Dimensions, values); err != nil {
		return q, err
	}
	sorts := q.Sort
	q.Sort = nil
	for _, s := range sorts {
		refs, err := j.expandBreakdowns([]sem.DimensionRef{{Name: s.Name}}, values)
		if err != nil {
			return q, err
		}
		for _, ref := range refs {
			q.Sort = append(q.Sort, sem.SortSpec{Name: ref.Name, Direction: s.Direction})
		}
	}
	return q, nil
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

// ResolveFilterSemanticJob uses the same engine and model aliases as widgets.
func (d *Dashboard) ResolveFilterSemanticJob(options *FilterOptions) (*SemanticJob, error) {
	w := &Widget{
		Dimension:  options.Dimension,
		Connection: options.Connection,
		Sort:       []SemanticSort{{Name: options.Dimension, Direction: "asc"}},
	}
	if options.Metric != nil {
		w.MetricRef = cmp.Or(options.Metric.Name, options.Metric.Field)
	}
	job, _, err := d.ResolveWidgetSemanticJob(w)
	if err == nil && job == nil {
		err = fmt.Errorf("options.dimension requires a semantic model")
	}
	return job, err
}
