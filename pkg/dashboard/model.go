package dashboard

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	sem "github.com/bruin-data/bruin/semantic-engine"
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
	Rows        []Row             `yaml:"rows" json:"rows"`

	// FilePath is the source file path, not serialized to JSON for API consumers.
	FilePath string `yaml:"-" json:"-"`

	// FileType indicates the source format: "yaml" or "tsx".
	FileType string `yaml:"-" json:"file_type,omitempty"`

	projectRoot     string                `yaml:"-" json:"-"`
	semanticModels  map[string]*sem.Model `yaml:"-" json:"-"`
	semanticInvalid map[string]error      `yaml:"-" json:"-"`
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

// Query represents a named query definition.
type Query struct {
	SQL        string                 `yaml:"sql,omitempty" json:"sql,omitempty"`
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
// Query resolution priority: query (named ref) > sql (inline) > data (static).
type Widget struct {
	ID          string `yaml:"id,omitempty" json:"id,omitempty"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Type        string `yaml:"type" json:"type"`
	Col         int    `yaml:"col,omitempty" json:"col,omitempty"`

	// Query source (pick one)
	QueryRef  string `yaml:"query,omitempty" json:"query,omitempty"` // reference to queries map key
	SQL       string `yaml:"sql,omitempty" json:"sql,omitempty"`
	MetricRef string `yaml:"metric,omitempty" json:"metric,omitempty"` // reference to metrics map key
	Model     string `yaml:"model,omitempty" json:"model,omitempty"`

	// Inline static data — an alternative to a query source. A widget with
	// Data renders without a connection or SQL; encoding fields (x/y/value/
	// label/columns) reference the column names in Data. Used for hardcoded
	// or sample dashboards, e.g. before a warehouse is connected.
	Data *WidgetData `yaml:"data,omitempty" json:"data,omitempty"`

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
	Chart      string         `yaml:"chart,omitempty" json:"chart,omitempty"` // line, bar, area, pie, scatter, bubble, combo, histogram, boxplot, funnel, sankey, heatmap, calendar, sparkline, waterfall, xmr, dumbbell, gauge, treemap, radar, candlestick, forest
	X          *AxisEncoding  `yaml:"x,omitempty" json:"x,omitempty"`
	Y          *AxisEncoding  `yaml:"y,omitempty" json:"y,omitempty"`
	// Y2 is an optional second value axis on the right (line/area/bar/combo): a
	// y-column plots against it when listed in y2.field, all others stay on y.
	Y2 *AxisEncoding `yaml:"y2,omitempty" json:"y2,omitempty"`
	Label      string         `yaml:"label,omitempty" json:"label,omitempty"` // for pie/funnel/treemap
	Value      *ValueEncoding `yaml:"value,omitempty" json:"value,omitempty"` // metric: the value; pie/funnel/heatmap/calendar/treemap/gauge: value column
	Color      *ColorEncoding `yaml:"color,omitempty" json:"color,omitempty"`
	Stacked    bool           `yaml:"stacked,omitempty" json:"stacked,omitempty"`
	Normalized bool           `yaml:"normalized,omitempty" json:"normalized,omitempty"`
	Horizontal *bool          `yaml:"horizontal,omitempty" json:"horizontal,omitempty"` // bar/funnel: horizontal layout; forest: defaults horizontal, set false for a vertical dot-and-whisker. Pointer so an explicit false survives JSON marshaling.
	Size       string         `yaml:"size,omitempty" json:"size,omitempty"`
	Source     string         `yaml:"source,omitempty" json:"source,omitempty"` // sankey: source column
	Target     string         `yaml:"target,omitempty" json:"target,omitempty"` // sankey: target column, gauge: target (max) column
	Bins       int            `yaml:"bins,omitempty" json:"bins,omitempty"`     // histogram: number of bins
	Lines      []string       `yaml:"lines,omitempty" json:"lines,omitempty"`   // combo: which y series render as lines
	// Series holds per-series line style overrides keyed by y-column: {column: {color, curve, dash}}.
	Series   map[string]SeriesStyle `yaml:"series,omitempty" json:"series,omitempty"`
	YMin     *BoundEncoding         `yaml:"yMin,omitempty" json:"yMin,omitempty"`         // xmr: min control limit column; line/bar/forest: CI lower bound (column or per-series map)
	YMax     *BoundEncoding         `yaml:"yMax,omitempty" json:"yMax,omitempty"`         // xmr: max control limit column; line/bar/forest: CI upper bound (column or per-series map)
	Open     string                 `yaml:"open,omitempty" json:"open,omitempty"`         // candlestick: open price column
	High     string                 `yaml:"high,omitempty" json:"high,omitempty"`         // candlestick: high price column
	Low      string                 `yaml:"low,omitempty" json:"low,omitempty"`           // candlestick: low price column
	Close    string                 `yaml:"close,omitempty" json:"close,omitempty"`       // candlestick: close price column
	RefLines []RefLine              `yaml:"refLines,omitempty" json:"refLines,omitempty"` // reference guide lines (axis + value + optional label)
	RefBands []RefBand              `yaml:"refBands,omitempty" json:"refBands,omitempty"` // shaded reference bands (axis + from/to + optional label)

	// Table fields
	Columns []TableColumn `yaml:"columns,omitempty" json:"columns,omitempty"`

	// Text fields
	Content string `yaml:"content,omitempty" json:"content,omitempty"`

	// Image fields
	Src string `yaml:"src,omitempty" json:"src,omitempty"`
	Alt string `yaml:"alt,omitempty" json:"alt,omitempty"`
}

// BoundEncoding is a CI bound (yMin/yMax): a single column name (scalar form) or a
// per-series map {series column: bound column} for multi-line CI bands.
type BoundEncoding struct {
	Field string            // scalar: a single bound column
	Map   map[string]string // per-series: {series column: bound column}
}

func (b *BoundEncoding) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Decode(&b.Field)
	case yaml.MappingNode:
		return node.Decode(&b.Map)
	default:
		return fmt.Errorf("yMin/yMax: must be a column name or a {series: column} map")
	}
}

func (b BoundEncoding) MarshalYAML() (any, error) {
	if len(b.Map) > 0 {
		return b.Map, nil
	}
	return b.Field, nil
}

func (b BoundEncoding) MarshalJSON() ([]byte, error) {
	if len(b.Map) > 0 {
		return json.Marshal(b.Map)
	}
	return json.Marshal(b.Field)
}

func (b *BoundEncoding) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		b.Field = s
		return nil
	}
	return json.Unmarshal(data, &b.Map)
}

// RefLine is a reference guide line on a chart's x or y axis.
type RefLine struct {
	Axis  string  `yaml:"axis" json:"axis"` // "x" | "y"
	Value float64 `yaml:"value" json:"value"`
	Label string  `yaml:"label,omitempty" json:"label,omitempty"`
	Color string  `yaml:"color,omitempty" json:"color,omitempty"`
}

// RefBand is a shaded reference band spanning from→to on a chart's x or y axis.
type RefBand struct {
	Axis  string  `yaml:"axis" json:"axis"` // "x" | "y"
	From  float64 `yaml:"from" json:"from"`
	To    float64 `yaml:"to" json:"to"`
	Label string  `yaml:"label,omitempty" json:"label,omitempty"`
	Color string  `yaml:"color,omitempty" json:"color,omitempty"`
}

type TableColumn struct {
	Name   string        `yaml:"name" json:"name"`
	Label  string        `yaml:"label,omitempty" json:"label,omitempty"`
	Number string        `yaml:"number,omitempty" json:"number,omitempty"` // value display: currency | number | d3-format spec
	Like   string        `yaml:"like,omitempty" json:"like,omitempty"`     // mirror another column's coloring + per-row value
	Hidden bool          `yaml:"hidden,omitempty" json:"hidden,omitempty"` // keep the column in the result (for cross-column rules / like) but don't render it
	Align  string        `yaml:"align,omitempty" json:"align,omitempty"`   // text alignment override: left | center | right (applies to the header and body cells)
	Format []FormatLayer `yaml:"format,omitempty" json:"format,omitempty"` // ordered conditional-format style layers; first match wins
}

// UnmarshalYAML makes a column's `format` polymorphic for backward compatibility.
// Originally `format` was a scalar value-display shorthand (`format: currency`);
// that role is now `number`, and `format` is an ordered list of style layers. So
// a scalar `format` is read as the legacy value format and folded into Number
// (unless Number is set explicitly); a list is decoded as the style layers. Value
// display (`number`, or a scalar `format`) and coloring (`format: [...]`) coexist.
func (c *TableColumn) UnmarshalYAML(node *yaml.Node) error {
	// Decode the plain fields, capturing `format` as a raw node (a nested struct
	// with no UnmarshalYAML, so this doesn't recurse).
	var tmp struct {
		Name   string    `yaml:"name"`
		Label  string    `yaml:"label,omitempty"`
		Number string    `yaml:"number,omitempty"`
		Like   string    `yaml:"like,omitempty"`
		Hidden bool      `yaml:"hidden,omitempty"`
		Align  string    `yaml:"align,omitempty"`
		Format yaml.Node `yaml:"format,omitempty"`
	}
	if err := node.Decode(&tmp); err != nil {
		return err
	}
	*c = TableColumn{Name: tmp.Name, Label: tmp.Label, Number: tmp.Number, Like: tmp.Like, Hidden: tmp.Hidden, Align: tmp.Align}

	// Follow a YAML alias to its target, then: a scalar is the legacy value-display
	// shorthand (folds into `number`); a list is the style layers.
	f := &tmp.Format
	for f.Kind == yaml.AliasNode && f.Alias != nil {
		f = f.Alias
	}
	switch f.Kind {
	case yaml.ScalarNode:
		if c.Number == "" {
			c.Number = f.Value
		}
	case yaml.SequenceNode:
		return f.Decode(&c.Format)
	}
	return nil
}

// Condition operators for conditional-formatting rules.
const (
	CondIsEmpty            = "is_empty"
	CondIsNotEmpty         = "is_not_empty"
	CondTextContains       = "text_contains"
	CondTextNotContains    = "text_does_not_contain"
	CondTextStartsWith     = "text_starts_with"
	CondTextEndsWith       = "text_ends_with"
	CondTextIsExactly      = "text_is_exactly"
	CondDateIs             = "date_is"
	CondDateBefore         = "date_before"
	CondDateAfter          = "date_after"
	CondGreaterThan        = "greater_than"
	CondGreaterThanOrEqual = "greater_than_or_equal"
	CondLessThan           = "less_than"
	CondLessThanOrEqual    = "less_than_or_equal"
	CondIsEqualTo          = "is_equal_to"
	CondIsNotEqualTo       = "is_not_equal_to"
	CondIsBetween          = "is_between"
	CondIsNotBetween       = "is_not_between"
)

// FormatLayer is one entry in a column's ordered `format` list; the first
// layer that matches a cell wins. A layer with `If` set is a condition (applies
// to matching cells). A layer without `If` always applies — a gradient (array
// backgroundColor, optional range/unit) or a flat fill (string backgroundColor);
// place it last as the fallback base.
type FormatLayer struct {
	If              string    `yaml:"if,omitempty" json:"if,omitempty"`
	Value           any       `yaml:"value,omitempty" json:"value,omitempty"`                     // scalar, [low, high], or {column: X}
	BackgroundColor any       `yaml:"backgroundColor,omitempty" json:"backgroundColor,omitempty"` // string (flat/single) | []string (gradient)
	Range           []float64 `yaml:"range,omitempty" json:"range,omitempty"`                     // gradient anchors, paired with Unit
	Unit            string    `yaml:"unit,omitempty" json:"unit,omitempty"`                       // absolute | percent | percentile
	TextColor       string    `yaml:"textColor,omitempty" json:"textColor,omitempty"`
	Bold            bool      `yaml:"bold,omitempty" json:"bold,omitempty"`
	Italic          bool      `yaml:"italic,omitempty" json:"italic,omitempty"`
	Underline       bool      `yaml:"underline,omitempty" json:"underline,omitempty"`
	Strikethrough   bool      `yaml:"strikethrough,omitempty" json:"strikethrough,omitempty"`
}

// WidgetData holds inline result data for a widget so it can render without a
// connection. Columns are column names; each entry in Rows is positional and
// must have one value per column. This mirrors the {columns, rows} shape the
// frontend uses for executed query results.
type WidgetData struct {
	Columns []string `yaml:"columns" json:"columns"`
	Rows    [][]any  `yaml:"rows" json:"rows"`
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

// HasInlineData reports whether the widget carries static inline data.
func (w *Widget) HasInlineData() bool {
	return w.Data != nil && len(w.Data.Columns) > 0
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
	if w.HasInlineData() {
		return "", "", nil // static widgets carry their data inline; nothing to execute
	}

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
		return q.SQL, conn, nil

	case w.SQL != "":
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

// SeriesStyle is a per-series style override, grouped under Widget.Series and
// keyed by the y-column the style applies to.
type SeriesStyle struct {
	Color string `yaml:"color,omitempty" json:"color,omitempty"` // #hex; unset uses the palette
	Curve string `yaml:"curve,omitempty" json:"curve,omitempty"` // smooth | straight | stepline (falls back to y.curve)
	Dash  string `yaml:"dash,omitempty" json:"dash,omitempty"`   // solid | dotted | dashed | long-dash (falls back to y.dash)
}

type AxisEncoding struct {
	Field  any    `yaml:"field" json:"field"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	Title  string `yaml:"title,omitempty" json:"title,omitempty"`
	Format string `yaml:"format,omitempty" json:"format,omitempty"`
	// BeginAtZero anchors a line/area chart's value axis at 0 (opt-in).
	BeginAtZero *bool `yaml:"beginAtZero,omitempty" json:"beginAtZero,omitempty"`
	// Markers toggles point markers on line/area charts (default on; false hides).
	Markers *bool `yaml:"markers,omitempty" json:"markers,omitempty"`
	// Curve is the chart-wide line interpolation: smooth | straight | stepline.
	Curve string `yaml:"curve,omitempty" json:"curve,omitempty"`
	// Dash is the chart-wide dash pattern every series inherits: dotted | dashed | long-dash (empty = solid).
	Dash string `yaml:"dash,omitempty" json:"dash,omitempty"`
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

func (w *Widget) Y2Fields() []string { return w.Y2.FieldList() }

const axisEncodingHint = "axis encoding: must be an object with a field key, e.g. x: { field: month } or y: { field: [revenue, cost] }"

func (a *AxisEncoding) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s", axisEncodingHint)
	}
	type axisFields struct {
		Field       yaml.Node `yaml:"field"`
		Type        string    `yaml:"type,omitempty"`
		Title       string    `yaml:"title,omitempty"`
		Format      string    `yaml:"format,omitempty"`
		BeginAtZero *bool     `yaml:"beginAtZero,omitempty"`
		Markers     *bool     `yaml:"markers,omitempty"`
		Curve       string    `yaml:"curve,omitempty"`
		Dash        string    `yaml:"dash,omitempty"`
	}
	var tmp axisFields
	if err := node.Decode(&tmp); err != nil {
		return err
	}
	a.Type = tmp.Type
	a.Title = tmp.Title
	a.Format = tmp.Format
	a.BeginAtZero = tmp.BeginAtZero
	a.Markers = tmp.Markers
	a.Curve = tmp.Curve
	a.Dash = tmp.Dash
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
		Field       any    `yaml:"field"`
		Type        string `yaml:"type,omitempty"`
		Title       string `yaml:"title,omitempty"`
		Format      string `yaml:"format,omitempty"`
		BeginAtZero *bool  `yaml:"beginAtZero,omitempty"`
		Markers     *bool  `yaml:"markers,omitempty"`
		Curve       string `yaml:"curve,omitempty"`
		Dash        string `yaml:"dash,omitempty"`
	}{a.Field, a.Type, a.Title, a.Format, a.BeginAtZero, a.Markers, a.Curve, a.Dash}, nil
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
		Field       json.RawMessage `json:"field"`
		Type        string          `json:"type,omitempty"`
		Title       string          `json:"title,omitempty"`
		Format      string          `json:"format,omitempty"`
		BeginAtZero *bool           `json:"beginAtZero,omitempty"`
		Markers     *bool           `json:"markers,omitempty"`
		Curve       string          `json:"curve,omitempty"`
		Dash        string          `json:"dash,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	a.Type = raw.Type
	a.Title = raw.Title
	a.Format = raw.Format
	a.BeginAtZero = raw.BeginAtZero
	a.Markers = raw.Markers
	a.Curve = raw.Curve
	a.Dash = raw.Dash
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
		Field       any    `json:"field"`
		Type        string `json:"type,omitempty"`
		Title       string `json:"title,omitempty"`
		Format      string `json:"format,omitempty"`
		BeginAtZero *bool  `json:"beginAtZero,omitempty"`
		Markers     *bool  `json:"markers,omitempty"`
		Curve       string `json:"curve,omitempty"`
		Dash        string `json:"dash,omitempty"`
	}{a.Field, a.Type, a.Title, a.Format, a.BeginAtZero, a.Markers, a.Curve, a.Dash})
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
	return "widget \"" + e.Widget + "\": no query or sql specified"
}
