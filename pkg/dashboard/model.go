package dashboard

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	sem "github.com/bruin-data/dac/pkg/semantic"
	"gopkg.in/yaml.v3"
)

// Widget type constants.
const (
	WidgetTypeMetric  = "metric"
	WidgetTypeChart   = "chart"
	WidgetTypeTable   = "table"
	WidgetTypeText    = "text"
	WidgetTypeDivider = "divider"
	WidgetTypeImage   = "image"
)

// Dashboard represents a complete dashboard definition loaded from YAML.
type Dashboard struct {
	Schema      string            `yaml:"schema,omitempty" json:"schema,omitempty"`
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Connection  string            `yaml:"connection,omitempty" json:"connection,omitempty"`
	Model       string            `yaml:"model,omitempty" json:"model,omitempty"`
	Models      map[string]string `yaml:"models,omitempty" json:"models,omitempty"`
	Filters     []Filter          `yaml:"filters,omitempty" json:"filters,omitempty"`
	Queries     map[string]Query  `yaml:"queries,omitempty" json:"queries,omitempty"`
	Semantic    *SemanticLayer    `yaml:"semantic,omitempty" json:"semantic,omitempty"`
	Rows        []Row             `yaml:"rows" json:"rows"`

	// FilePath is the source file path, not serialized to JSON for API consumers.
	FilePath string `yaml:"-" json:"-"`

	// FileType indicates the source format: "yaml" or "tsx".
	FileType string `yaml:"-" json:"file_type,omitempty"`

	projectRoot     string                `yaml:"-" json:"-"`
	semanticModels  map[string]*sem.Model `yaml:"-" json:"-"`
	semanticInvalid map[string]error      `yaml:"-" json:"-"`
}

// SemanticLayer groups the declarative source, metrics, and dimensions.
type SemanticLayer struct {
	Source     *Source              `yaml:"source,omitempty" json:"source,omitempty"`
	Metrics    map[string]Metric    `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Dimensions map[string]Dimension `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
}

type Filter struct {
	Name     string         `yaml:"name" json:"name"`
	Type     string         `yaml:"type" json:"type"`
	Multiple bool           `yaml:"multiple,omitempty" json:"multiple,omitempty"`
	Default  any            `yaml:"default,omitempty" json:"default,omitempty"`
	Options  *FilterOptions `yaml:"options,omitempty" json:"options,omitempty"`
}

type FilterOptions struct {
	Values     []string `yaml:"values,omitempty" json:"values,omitempty"`
	Query      string   `yaml:"query,omitempty" json:"query,omitempty"`
	Connection string   `yaml:"connection,omitempty" json:"connection,omitempty"`
	Presets    []string `yaml:"presets,omitempty" json:"presets,omitempty"` // date-range: which presets to show
}

// Source defines the base table for declarative metrics.
type Source struct {
	Table      string `yaml:"table" json:"table"`
	DateColumn string `yaml:"date_column,omitempty" json:"date_column,omitempty"`
	DateFormat string `yaml:"date_format,omitempty" json:"date_format,omitempty"`
	Connection string `yaml:"connection,omitempty" json:"connection,omitempty"`
}

// Metric defines a named scalar value computed from the source table.
// Use Aggregate for database-computed metrics, or Expression for
// client-side arithmetic over other metrics.
type Metric struct {
	Aggregate  string            `yaml:"aggregate,omitempty" json:"aggregate,omitempty"` // count, count_distinct, sum, avg, min, max
	Column     string            `yaml:"column,omitempty" json:"column,omitempty"`
	Filter     map[string]string `yaml:"filter,omitempty" json:"filter,omitempty"`
	Expression string            `yaml:"expression,omitempty" json:"expression,omitempty"`
}

// IsExpression returns true if this metric is computed from other metrics.
func (m *Metric) IsExpression() bool {
	return m.Expression != ""
}

// Dimension defines a named grouping column for dimensional queries.
type Dimension struct {
	Column string `yaml:"column" json:"column"`                 // the SQL column expression (e.g. "geo.country")
	Type   string `yaml:"type,omitempty" json:"type,omitempty"` // "date" for chronological ordering, empty for top-N
}

// IsDate returns true if this dimension represents a date/time column.
func (dim *Dimension) IsDate() bool {
	return dim.Type == "date"
}

// Query represents a named query definition.
type Query struct {
	SQL        string                 `yaml:"sql,omitempty" json:"sql,omitempty"`
	File       string                 `yaml:"file,omitempty" json:"file,omitempty"`
	Connection string                 `yaml:"connection,omitempty" json:"connection,omitempty"`
	Model      string                 `yaml:"model,omitempty" json:"model,omitempty"`
	Dimensions []SemanticDimensionRef `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	Metrics    []string               `yaml:"metrics,omitempty" json:"metrics,omitempty"`
	Filters    []SemanticQueryFilter  `yaml:"filters,omitempty" json:"filters,omitempty"`
	Segments   []string               `yaml:"segments,omitempty" json:"segments,omitempty"`
	Sort       []SemanticSort         `yaml:"sort,omitempty" json:"sort,omitempty"`
	Limit      int                    `yaml:"limit,omitempty" json:"limit,omitempty"`
}

type Row struct {
	Tab     string   `yaml:"tab,omitempty" json:"tab,omitempty"`
	Height  any      `yaml:"height,omitempty" json:"height,omitempty"`
	Widgets []Widget `yaml:"widgets" json:"widgets"`
}

// Widget represents a single dashboard widget.
// Query resolution priority: query (named ref) > sql (inline) > file (external).
type Widget struct {
	ID          string `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string `yaml:"type" json:"type"`
	Col         int    `yaml:"col,omitempty" json:"col,omitempty"`

	// Query source (pick one)
	QueryRef  string `yaml:"query,omitempty" json:"query,omitempty"` // reference to queries map key
	SQL       string `yaml:"sql,omitempty" json:"sql,omitempty"`
	File      string `yaml:"file,omitempty" json:"file,omitempty"`
	MetricRef string `yaml:"metric,omitempty" json:"metric,omitempty"` // reference to metrics map key
	Model     string `yaml:"model,omitempty" json:"model,omitempty"`

	// Connection override for inline queries
	Connection string `yaml:"connection,omitempty" json:"connection,omitempty"`

	// Declarative chart fields (use with source + metrics)
	Dimension   string                 `yaml:"dimension,omitempty" json:"dimension,omitempty"` // GROUP BY column
	Granularity string                 `yaml:"granularity,omitempty" json:"granularity,omitempty"`
	Dimensions  []SemanticDimensionRef `yaml:"dimensions,omitempty" json:"dimensions,omitempty"`
	MetricRefs  []string               `yaml:"metrics,omitempty" json:"metrics,omitempty"` // metric names to aggregate
	Filters     []SemanticQueryFilter  `yaml:"filters,omitempty" json:"filters,omitempty"`
	Segments    []string               `yaml:"segments,omitempty" json:"segments,omitempty"`
	Sort        []SemanticSort         `yaml:"sort,omitempty" json:"sort,omitempty"`
	Limit       int                    `yaml:"limit,omitempty" json:"limit,omitempty"` // LIMIT for dimensional queries

	// Chart fields
	Chart      string         `yaml:"chart,omitempty" json:"chart,omitempty"` // line, bar, area, pie, scatter, bubble, combo, histogram, boxplot, funnel, sankey, heatmap, calendar, sparkline, waterfall, xmr, dumbbell, gauge, treemap, radar, candlestick
	X          *AxisEncoding  `yaml:"x,omitempty" json:"x,omitempty"`
	Y          *AxisEncoding  `yaml:"y,omitempty" json:"y,omitempty"`
	Label      string         `yaml:"label,omitempty" json:"label,omitempty"` // for pie/funnel/treemap
	Value      *ValueEncoding `yaml:"value,omitempty" json:"value,omitempty"` // metric: the value; pie/funnel/heatmap/calendar/treemap/gauge: value column
	Color      *ColorEncoding `yaml:"color,omitempty" json:"color,omitempty"`
	Stacked    bool           `yaml:"stacked,omitempty" json:"stacked,omitempty"`
	Normalized bool           `yaml:"normalized,omitempty" json:"normalized,omitempty"`
	Horizontal bool           `yaml:"horizontal,omitempty" json:"horizontal,omitempty"`
	Size       string         `yaml:"size,omitempty" json:"size,omitempty"`
	Source     string         `yaml:"source,omitempty" json:"source,omitempty"` // sankey: source column
	Target     string         `yaml:"target,omitempty" json:"target,omitempty"` // sankey: target column, gauge: target (max) column
	Bins       int            `yaml:"bins,omitempty" json:"bins,omitempty"`     // histogram: number of bins
	Lines      []string       `yaml:"lines,omitempty" json:"lines,omitempty"`   // combo: which y series render as lines
	YMin       string         `yaml:"yMin,omitempty" json:"yMin,omitempty"`     // xmr: min control limit column
	YMax       string         `yaml:"yMax,omitempty" json:"yMax,omitempty"`     // xmr: max control limit column
	Open       string         `yaml:"open,omitempty" json:"open,omitempty"`     // candlestick: open price column
	High       string         `yaml:"high,omitempty" json:"high,omitempty"`     // candlestick: high price column
	Low        string         `yaml:"low,omitempty" json:"low,omitempty"`       // candlestick: low price column
	Close      string         `yaml:"close,omitempty" json:"close,omitempty"`   // candlestick: close price column

	// Table fields
	Columns []TableColumn `yaml:"columns,omitempty" json:"columns,omitempty"`

	// Text fields
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Image fields
	Src string `yaml:"src,omitempty" json:"src,omitempty"`
	Alt string `yaml:"alt,omitempty" json:"alt,omitempty"`
}

type TableColumn struct {
	Name   string `yaml:"name" json:"name"`
	Label  string `yaml:"label,omitempty" json:"label,omitempty"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
}

type SemanticDimensionRef struct {
	Name        string `yaml:"name" json:"name"`
	Granularity string `yaml:"granularity,omitempty" json:"granularity,omitempty"`
}

type SemanticQueryFilter struct {
	Dimension  string `yaml:"dimension,omitempty" json:"dimension,omitempty"`
	Operator   string `yaml:"operator,omitempty" json:"operator,omitempty"`
	Value      any    `yaml:"value,omitempty" json:"value,omitempty"`
	Expression string `yaml:"expression,omitempty" json:"expression,omitempty"`
}

type SemanticSort struct {
	Name      string `yaml:"name" json:"name"`
	Direction string `yaml:"direction,omitempty" json:"direction,omitempty"`
}

// DefaultFilters returns a map of filter names to their default values.
// For date-range filters, string defaults like "last_30_days" are resolved
// to {start, end} maps so that query templating works correctly.
func (d *Dashboard) DefaultFilters() map[string]any {
	defaults := make(map[string]any)
	for _, f := range d.Filters {
		if f.Default != nil {
			val := f.Default
			if f.Type == "date-range" {
				if preset, ok := val.(string); ok {
					if resolved := ResolveDatePreset(preset); resolved != nil {
						val = resolved
					}
				}
			}
			if f.Type == "date" {
				if t, ok := val.(time.Time); ok {
					val = t.Format("2006-01-02")
				} else if expr, ok := val.(string); ok {
					if resolved := ResolveDateExpression(expr); resolved != "" {
						val = resolved
					}
				}
			}
			defaults[f.Name] = val
		}
	}
	return defaults
}

var DateExpressionPattern = regexp.MustCompile(`^(TODAY([+-]\d+)?|\d{4}-\d{2}-\d{2})$`)

func ResolveDateExpression(expr string) string {
	if !DateExpressionPattern.MatchString(expr) {
		return ""
	}
	if expr == "TODAY" {
		return time.Now().Format("2006-01-02")
	}
	if strings.HasPrefix(expr, "TODAY") {
		n, err := strconv.Atoi(expr[len("TODAY"):])
		if err != nil {
			return ""
		}
		return time.Now().AddDate(0, 0, n).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", expr); err != nil {
		return ""
	}
	return expr
}

// ResolveDatePreset converts a preset key like "last_30_days" into a
// map with "start" and "end" date strings. Returns nil if the key is unknown.
func ResolveDatePreset(key string) map[string]any {
	now := time.Now()
	today := now.Format("2006-01-02")

	switch key {
	case "today":
		return map[string]any{"start": today, "end": today}
	case "yesterday":
		d := now.AddDate(0, 0, -1).Format("2006-01-02")
		return map[string]any{"start": d, "end": d}
	case "last_7_days":
		return map[string]any{"start": now.AddDate(0, 0, -6).Format("2006-01-02"), "end": today}
	case "last_30_days":
		return map[string]any{"start": now.AddDate(0, 0, -29).Format("2006-01-02"), "end": today}
	case "last_90_days":
		return map[string]any{"start": now.AddDate(0, 0, -89).Format("2006-01-02"), "end": today}
	case "this_month":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, -1)
		return map[string]any{"start": start.Format("2006-01-02"), "end": end.Format("2006-01-02")}
	case "last_month":
		start := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location())
		end := time.Date(now.Year(), now.Month(), 0, 0, 0, 0, 0, now.Location())
		return map[string]any{"start": start.Format("2006-01-02"), "end": end.Format("2006-01-02")}
	case "this_quarter":
		q := (int(now.Month()) - 1) / 3
		start := time.Date(now.Year(), time.Month(q*3+1), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 3, -1)
		return map[string]any{"start": start.Format("2006-01-02"), "end": end.Format("2006-01-02")}
	case "this_year":
		return map[string]any{"start": fmt.Sprintf("%d-01-01", now.Year()), "end": fmt.Sprintf("%d-12-31", now.Year())}
	case "year_to_date":
		return map[string]any{"start": fmt.Sprintf("%d-01-01", now.Year()), "end": today}
	case "all_time":
		return map[string]any{"start": "1970-01-01", "end": "2099-12-31"}
	default:
		return nil
	}
}

// DateRangeFilterName returns the name of the first date-range filter, or "".
func (d *Dashboard) DateRangeFilterName() string {
	for _, f := range d.Filters {
		if f.Type == "date-range" {
			return f.Name
		}
	}
	return ""
}

// SourceConnection returns the connection for the semantic source, falling
// back to the dashboard default.
func (d *Dashboard) SourceConnection() string {
	if d.Semantic != nil && d.Semantic.Source != nil && d.Semantic.Source.Connection != "" {
		return d.Semantic.Source.Connection
	}
	return d.Connection
}

// SemanticSource returns the semantic layer's source, or nil.
func (d *Dashboard) SemanticSource() *Source {
	if d.Semantic != nil {
		return d.Semantic.Source
	}
	return nil
}

// SemanticMetrics returns the semantic layer's metrics, or nil.
func (d *Dashboard) SemanticMetrics() map[string]Metric {
	if d.Semantic != nil {
		return d.Semantic.Metrics
	}
	return nil
}

// SemanticDimensions returns the semantic layer's dimensions, or nil.
func (d *Dashboard) SemanticDimensions() map[string]Dimension {
	if d.Semantic != nil {
		return d.Semantic.Dimensions
	}
	return nil
}

func (d *Dashboard) SetProjectContext(projectRoot string, semanticModels map[string]*sem.Model, semanticInvalid map[string]error) {
	d.projectRoot = projectRoot
	d.semanticModels = semanticModels
	d.semanticInvalid = semanticInvalid
}

func (d *Dashboard) ResolveSemanticModel(ref string) (*sem.Model, string, error) {
	name := strings.TrimSpace(ref)
	if name == "" {
		name = strings.TrimSpace(d.Model)
	}
	if actual, ok := d.Models[name]; ok {
		name = actual
	}
	if name == "" {
		return nil, "", nil
	}
	model, ok := d.semanticModels[name]
	if !ok {
		if err, invalid := d.semanticInvalid[name]; invalid {
			return nil, "", fmt.Errorf("semantic model %q is invalid: %w", name, err)
		}
		return nil, "", fmt.Errorf("semantic model %q not found", name)
	}
	return model, name, nil
}

func (q *Query) IsSemantic() bool {
	return q.Model != "" ||
		len(q.Dimensions) > 0 ||
		len(q.Metrics) > 0 ||
		len(q.Filters) > 0 ||
		len(q.Segments) > 0 ||
		len(q.Sort) > 0 ||
		q.Limit > 0
}

func (w *Widget) IsSemantic() bool {
	return w.Model != "" ||
		len(w.Dimensions) > 0 ||
		w.Dimension != "" ||
		w.Granularity != "" ||
		len(w.MetricRefs) > 0 ||
		len(w.Filters) > 0 ||
		len(w.Segments) > 0 ||
		len(w.Sort) > 0
}

// FindByName returns the dashboard with the given name from a slice, or nil.
func FindByName(dashboards []*Dashboard, name string) *Dashboard {
	for _, d := range dashboards {
		if d.Name == name {
			return d
		}
	}
	return nil
}

// ResolvedQuery returns the SQL and connection for this widget, resolving named query references.
// Widgets with MetricRef are handled separately and should not call this method.
func (w *Widget) ResolvedQuery(dashboard *Dashboard) (sql, connection string, err error) {
	if w.MetricRef != "" || len(w.MetricRefs) > 0 {
		return "", "", nil // metric-based widgets are resolved via the metrics system
	}

	switch {
	case w.QueryRef != "":
		q, ok := dashboard.Queries[w.QueryRef]
		if !ok {
			return "", "", &QueryNotFoundError{Name: w.QueryRef, Widget: w.Name}
		}
		conn := q.Connection
		if conn == "" {
			conn = dashboard.Connection
		}
		if q.SQL != "" {
			return q.SQL, conn, nil
		}
		// File-based query — SQL should have been loaded by the loader.
		return q.SQL, conn, nil

	case w.SQL != "":
		conn := w.Connection
		if conn == "" {
			conn = dashboard.Connection
		}
		return w.SQL, conn, nil

	case w.File != "":
		// File-based inline query — SQL should have been loaded by the loader.
		conn := w.Connection
		if conn == "" {
			conn = dashboard.Connection
		}
		return w.SQL, conn, nil

	default:
		if w.Type == WidgetTypeText || w.Type == WidgetTypeDivider || w.Type == WidgetTypeImage {
			return "", "", nil
		}
		return "", "", &NoQueryError{Widget: w.Name}
	}
}

type AxisEncoding struct {
	Field  any    `yaml:"field" json:"field"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Title  string `yaml:"title,omitempty" json:"title,omitempty"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
}

func (a *AxisEncoding) FieldString() string {
	if a == nil || a.Field == nil {
		return ""
	}
	switch v := a.Field.(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func (a *AxisEncoding) FieldList() []string {
	if a == nil || a.Field == nil {
		return nil
	}
	switch v := a.Field.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	}
	return nil
}

func (w *Widget) XField() string { return w.X.FieldString() }

func (w *Widget) YFields() []string { return w.Y.FieldList() }

const axisEncodingHint = "axis encoding: must be an object with a field key, e.g. x: { field: month } or y: { field: [revenue, cost] }"

func (a *AxisEncoding) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s", axisEncodingHint)
	}
	type axisFields struct {
		Field  yaml.Node `yaml:"field"`
		Type   string    `yaml:"type,omitempty"`
		Title  string    `yaml:"title,omitempty"`
		Format string    `yaml:"format,omitempty"`
	}
	var tmp axisFields
	if err := node.Decode(&tmp); err != nil {
		return err
	}
	a.Type = tmp.Type
	a.Title = tmp.Title
	a.Format = tmp.Format
	switch tmp.Field.Kind {
	case yaml.ScalarNode:
		var s string
		if err := tmp.Field.Decode(&s); err != nil {
			return err
		}
		a.Field = s
	case yaml.SequenceNode:
		var list []string
		if err := tmp.Field.Decode(&list); err != nil {
			return err
		}
		a.Field = list
	case 0:
		return fmt.Errorf("axis encoding: field is required")
	default:
		return fmt.Errorf("axis encoding: field must be a string or list of strings")
	}
	return nil
}

func (a *AxisEncoding) MarshalYAML() (any, error) {
	if a == nil {
		return nil, nil
	}
	return struct {
		Field  any    `yaml:"field"`
		Type   string `yaml:"type,omitempty"`
		Title  string `yaml:"title,omitempty"`
		Format string `yaml:"format,omitempty"`
	}{a.Field, a.Type, a.Title, a.Format}, nil
}

func (a *AxisEncoding) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("%s", axisEncodingHint)
	}
	var raw struct {
		Field  json.RawMessage `json:"field"`
		Type   string          `json:"type,omitempty"`
		Title  string          `json:"title,omitempty"`
		Format string          `json:"format,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Type = raw.Type
	a.Title = raw.Title
	a.Format = raw.Format
	fieldStr := strings.TrimSpace(string(raw.Field))
	if fieldStr == "" || fieldStr == "null" {
		return fmt.Errorf("axis encoding: field is required")
	}
	if fieldStr[0] == '[' {
		var list []string
		if err := json.Unmarshal(raw.Field, &list); err != nil {
			return err
		}
		a.Field = list
		return nil
	}
	var s string
	if err := json.Unmarshal(raw.Field, &s); err != nil {
		return fmt.Errorf("axis encoding: field must be a string or list of strings")
	}
	a.Field = s
	return nil
}

func (a *AxisEncoding) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Field  any    `json:"field"`
		Type   string `json:"type,omitempty"`
		Title  string `json:"title,omitempty"`
		Format string `json:"format,omitempty"`
	}{a.Field, a.Type, a.Title, a.Format})
}

func newAxisField(s string) *AxisEncoding {
	if s == "" {
		return nil
	}
	return &AxisEncoding{Field: s}
}

func newAxisFields(list []string) *AxisEncoding {
	if len(list) == 0 {
		return nil
	}
	return &AxisEncoding{Field: list}
}

type ValueEncoding struct {
	Field  string `yaml:"field" json:"field"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
}

func (v *ValueEncoding) FieldString() string {
	if v == nil {
		return ""
	}
	return v.Field
}

func (w *Widget) ValueField() string { return w.Value.FieldString() }

const valueEncodingHint = "value encoding: must be an object with a field key, e.g. value: { field: revenue }"

func (v *ValueEncoding) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s", valueEncodingHint)
	}
	type valueFields struct {
		Field  string `yaml:"field"`
		Type   string `yaml:"type,omitempty"`
		Format string `yaml:"format,omitempty"`
	}
	var tmp valueFields
	if err := node.Decode(&tmp); err != nil {
		return err
	}
	if tmp.Field == "" {
		return fmt.Errorf("value encoding: field is required")
	}
	v.Field = tmp.Field
	v.Type = tmp.Type
	v.Format = tmp.Format
	return nil
}

func (v *ValueEncoding) MarshalYAML() (any, error) {
	if v == nil {
		return nil, nil
	}
	return struct {
		Field  string `yaml:"field"`
		Type   string `yaml:"type,omitempty"`
		Format string `yaml:"format,omitempty"`
	}{v.Field, v.Type, v.Format}, nil
}

func (v *ValueEncoding) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	if trimmed[0] != '{' {
		return fmt.Errorf("%s", valueEncodingHint)
	}
	var raw struct {
		Field  string `json:"field"`
		Type   string `json:"type,omitempty"`
		Format string `json:"format,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Field == "" {
		return fmt.Errorf("value encoding: field is required")
	}
	v.Field = raw.Field
	v.Type = raw.Type
	v.Format = raw.Format
	return nil
}

func (v *ValueEncoding) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(struct {
		Field  string `json:"field"`
		Type   string `json:"type,omitempty"`
		Format string `json:"format,omitempty"`
	}{v.Field, v.Type, v.Format})
}

type ColorEncoding struct {
	Field string `yaml:"field" json:"field"`
}

func (c *ColorEncoding) FieldString() string {
	if c == nil {
		return ""
	}
	return c.Field
}

func (w *Widget) ColorField() string { return w.Color.FieldString() }

type QueryNotFoundError struct {
	Name   string
	Widget string
}

func (e *QueryNotFoundError) Error() string {
	return "widget \"" + e.Widget + "\": query \"" + e.Name + "\" not found in queries map"
}

type NoQueryError struct {
	Widget string
}

func (e *NoQueryError) Error() string {
	return "widget \"" + e.Widget + "\": no query, sql, or file specified"
}
