package dashboard

import (
	"fmt"
	"strings"
	"time"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

// ValidationError holds all validation issues for a dashboard.
type ValidationError struct {
	Dashboard string
	Errors    []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("dashboard %q has %d validation error(s):\n  - %s",
		e.Dashboard, len(e.Errors), strings.Join(e.Errors, "\n  - "))
}

type ValidationSetError struct {
	Errors []error
}

func (e *ValidationSetError) Error() string {
	if len(e.Errors) == 0 {
		return ""
	}
	var parts []string
	for _, err := range e.Errors {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n\n")
}

// Validate checks a dashboard definition for correctness.
func Validate(d *Dashboard) error {
	var errs []string

	if d.Name == "" {
		errs = append(errs, "name is required")
	}

	if len(d.Rows) == 0 {
		errs = append(errs, "at least one row is required")
	}

	for i, row := range d.Rows {
		if len(row.Widgets) == 0 {
			errs = append(errs, fmt.Sprintf("row %d: at least one widget is required", i+1))
			continue
		}

		totalCols := 0
		for j, w := range row.Widgets {
			prefix := fmt.Sprintf("row %d, widget %d (%q)", i+1, j+1, w.Name)

			if w.Name == "" {
				errs = append(errs, fmt.Sprintf("row %d, widget %d: name is required", i+1, j+1))
			}

			if w.Type == "" {
				errs = append(errs, fmt.Sprintf("%s: type is required", prefix))
			}

			// Validate widget type.
			switch w.Type {
			case WidgetTypeMetric:
				errs = append(errs, validateMetricWidget(prefix, &w, d)...)
			case WidgetTypeChart:
				errs = append(errs, validateChartWidget(prefix, &w, d)...)
			case WidgetTypeTable:
				// Table widgets just need a query source.
				errs = append(errs, validateQuerySource(prefix, &w, d)...)
				validateTableColumns(prefix, &w, &errs)
			case WidgetTypeText:
				if w.Content == "" {
					errs = append(errs, fmt.Sprintf("%s: content is required for text widgets", prefix))
				}
			case WidgetTypeDivider:
				// No required fields.
			case WidgetTypeImage:
				if w.Src == "" {
					errs = append(errs, fmt.Sprintf("%s: src is required for image widgets", prefix))
				}
			case "":
				// Already reported above.
			default:
				errs = append(errs, fmt.Sprintf("%s: unknown widget type %q (expected metric, chart, table, text, divider, or image)", prefix, w.Type))
			}

			errs = append(errs, validateInlineData(prefix, &w)...)

			if w.Col < 0 || w.Col > 12 {
				errs = append(errs, fmt.Sprintf("%s: col must be between 1 and 12, got %d", prefix, w.Col))
			}
			if w.Col > 0 {
				totalCols += w.Col
			}
		}

		if totalCols > 12 {
			errs = append(errs, fmt.Sprintf("row %d: total column span is %d, exceeds 12", i+1, totalCols))
		}
	}

	// Validate filters.
	for i, f := range d.Filters {
		prefix := fmt.Sprintf("filter %d (%q)", i+1, f.Name)
		if f.Name == "" {
			errs = append(errs, fmt.Sprintf("filter %d: name is required", i+1))
		}
		if f.Type == "" {
			errs = append(errs, fmt.Sprintf("%s: type is required", prefix))
		}
		validTypes := map[string]bool{"date": true, "date-range": true, "number": true, "select": true, "text": true}
		if f.Type != "" && !validTypes[f.Type] {
			errs = append(errs, fmt.Sprintf("%s: unknown filter type %q", prefix, f.Type))
		}
		if f.Type == "date" && f.Default != nil {
			switch v := f.Default.(type) {
			case string:
				if ResolveDateExpression(v) == "" {
					errs = append(errs, fmt.Sprintf("%s: invalid date default %q (must be TODAY, TODAY+/-N, or YYYY-MM-DD)", prefix, v))
				}
			case time.Time:
			default:
				errs = append(errs, fmt.Sprintf("%s: invalid date default %v (must be TODAY, TODAY+/-N, or YYYY-MM-DD)", prefix, f.Default))
			}
		}
	}

	if d.Model != "" {
		if _, _, err := d.ResolveSemanticModel(d.Model); err != nil {
			errs = append(errs, err.Error())
		}
	}
	for alias, modelName := range d.Models {
		if _, _, err := d.ResolveSemanticModel(alias); err != nil {
			errs = append(errs, fmt.Sprintf("models %q: %v", alias, err))
		}
		if modelName == "" {
			errs = append(errs, fmt.Sprintf("models %q: semantic model name is required", alias))
		}
	}
	for name, q := range d.Queries {
		if !q.IsSemantic() {
			continue
		}
		if err := validateSemanticNamedQuery(d, q); err != nil {
			errs = append(errs, fmt.Sprintf("query %q: %v", name, err))
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Dashboard: d.Name, Errors: errs}
	}
	return nil
}

func ValidateAll(dashboards []*Dashboard) error {
	var errs []error
	for _, d := range dashboards {
		if err := Validate(d); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return &ValidationSetError{Errors: errs}
}

// validateInlineData checks a widget's static inline data, when present: it is
// only valid on data-bearing widget types, cannot be combined with a query
// source, and every row must line up with the declared columns.
func validateInlineData(prefix string, w *Widget) []string {
	if w.Data == nil {
		return nil
	}

	var errs []string

	switch w.Type {
	case WidgetTypeMetric, WidgetTypeChart, WidgetTypeTable:
	default:
		return append(errs, fmt.Sprintf("%s: data is only valid on metric, chart, or table widgets", prefix))
	}

	if w.SQL != "" || w.QueryRef != "" || w.IsSemantic() {
		errs = append(errs, fmt.Sprintf("%s: data cannot be combined with sql, query, or semantic fields", prefix))
	}

	if len(w.Data.Columns) == 0 {
		errs = append(errs, fmt.Sprintf("%s: data.columns is required and must be non-empty", prefix))
		return errs
	}

	seen := make(map[string]bool, len(w.Data.Columns))
	for _, c := range w.Data.Columns {
		if c == "" {
			errs = append(errs, fmt.Sprintf("%s: data.columns must not contain empty column names", prefix))
		} else if seen[c] {
			errs = append(errs, fmt.Sprintf("%s: data.columns has duplicate column %q", prefix, c))
		}
		seen[c] = true
	}

	for i, row := range w.Data.Rows {
		if len(row) != len(w.Data.Columns) {
			errs = append(errs, fmt.Sprintf("%s: data.rows[%d] has %d value(s), expected %d to match columns", prefix, i, len(row), len(w.Data.Columns)))
		}
	}

	return errs
}

func validateQuerySource(prefix string, w *Widget, d *Dashboard) []string {
	var errs []string
	if job, handled, err := d.ResolveWidgetSemanticJob(w); err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
		return errs
	} else if handled {
		if err := validateSemanticJob(job); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
		}
		return errs
	}
	if w.QueryRef == "" && w.SQL == "" && !w.HasInlineData() {
		errs = append(errs, fmt.Sprintf("%s: one of query, sql, or data is required", prefix))
	}
	if w.QueryRef != "" {
		if _, ok := d.Queries[w.QueryRef]; !ok {
			errs = append(errs, fmt.Sprintf("%s: query %q not found in queries map", prefix, w.QueryRef))
		}
	}
	return errs
}

func validateMetricWidget(prefix string, w *Widget, d *Dashboard) []string {
	var errs []string
	if job, handled, err := d.ResolveWidgetSemanticJob(w); err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
		return errs
	} else if handled {
		if err := validateSemanticJob(job); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
		}
		return errs
	}
	if w.MetricRef != "" {
		errs = append(errs, fmt.Sprintf("%s: metric %q requires a semantic model; set model on the widget or dashboard", prefix, w.MetricRef))
		return errs
	}
	if w.ValueField() == "" {
		errs = append(errs, fmt.Sprintf("%s: value is required for metric widgets", prefix))
	}
	return errs
}

var validChartTypes = map[string]bool{
	"line": true, "bar": true, "area": true, "pie": true,
	"scatter": true, "bubble": true, "combo": true, "histogram": true,
	"boxplot": true, "funnel": true, "sankey": true, "heatmap": true,
	"calendar": true, "sparkline": true, "waterfall": true, "xmr": true,
	"dumbbell": true, "gauge": true, "treemap": true, "radar": true,
	"candlestick": true, "forest": true,
}

// validCurves is the allowed set for y.curve and per-series y.curves values.
var validCurves = map[string]bool{"smooth": true, "straight": true, "stepline": true}

// validDashes is the allowed set for y.dash and per-series dash values
// ('solid' is a valid per-series override that forces a solid line).
var validDashes = map[string]bool{"solid": true, "dotted": true, "dashed": true, "long-dash": true}

// isHexColor reports whether s is a #rgb or #rrggbb colour.
func isHexColor(s string) bool {
	if (len(s) != 4 && len(s) != 7) || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// validateRefAnnotations checks widget.refLines / widget.refBands: axis must be
// x or y, and a band's range must be non-empty.
func validateRefAnnotations(prefix string, w *Widget) []string {
	var errs []string
	// Reference lines/bands only render on the cartesian charts that draw them;
	// on other types they would validate but silently do nothing.
	if len(w.RefLines) > 0 || len(w.RefBands) > 0 {
		switch w.Chart {
		case "line", "area", "bar", "forest":
		default:
			errs = append(errs, fmt.Sprintf("%s: refLines/refBands are only supported on line, area, bar and forest charts, got %q", prefix, w.Chart))
		}
	}
	// xmr control limits are a single column per bound; a per-series map would be
	// silently ignored by the renderer.
	if w.Chart == "xmr" {
		if w.YMin != nil && len(w.YMin.Map) > 0 {
			errs = append(errs, fmt.Sprintf("%s: xmr yMin must be a single column, not a per-series map", prefix))
		}
		if w.YMax != nil && len(w.YMax.Map) > 0 {
			errs = append(errs, fmt.Sprintf("%s: xmr yMax must be a single column, not a per-series map", prefix))
		}
	}
	for i, l := range w.RefLines {
		if l.Axis != "x" && l.Axis != "y" {
			errs = append(errs, fmt.Sprintf("%s: refLines[%d].axis must be \"x\" or \"y\", got %q", prefix, i, l.Axis))
		}
	}
	for i, b := range w.RefBands {
		if b.Axis != "x" && b.Axis != "y" {
			errs = append(errs, fmt.Sprintf("%s: refBands[%d].axis must be \"x\" or \"y\", got %q", prefix, i, b.Axis))
		}
		if b.From == b.To {
			errs = append(errs, fmt.Sprintf("%s: refBands[%d] from and to must differ", prefix, i))
		}
	}
	return errs
}

func validateChartWidget(prefix string, w *Widget, d *Dashboard) []string {
	var errs []string
	if w.Chart == "" {
		errs = append(errs, fmt.Sprintf("%s: chart type is required", prefix))
		return errs
	}
	if !validChartTypes[w.Chart] {
		errs = append(errs, fmt.Sprintf("%s: unknown chart type %q", prefix, w.Chart))
		return errs
	}
	errs = append(errs, validateRefAnnotations(prefix, w)...)

	// Chart-wide line style lives on y; per-series overrides live on the widget
	// (widget.series), keyed by y-column.
	if w.Y != nil {
		if w.Y.Curve != "" && !validCurves[w.Y.Curve] {
			errs = append(errs, fmt.Sprintf("%s: y.curve must be smooth, straight, or stepline", prefix))
		}
		if w.Y.Dash != "" && !validDashes[w.Y.Dash] {
			errs = append(errs, fmt.Sprintf("%s: y.dash must be solid, dotted, dashed, or long-dash", prefix))
		}
	}
	for col, st := range w.Series {
		if st.Curve != "" && !validCurves[st.Curve] {
			errs = append(errs, fmt.Sprintf("%s: series[%q].curve must be smooth, straight, or stepline", prefix, col))
		}
		if st.Dash != "" && !validDashes[st.Dash] {
			errs = append(errs, fmt.Sprintf("%s: series[%q].dash must be solid, dotted, dashed, or long-dash", prefix, col))
		}
		if st.Color != "" && !isHexColor(st.Color) {
			errs = append(errs, fmt.Sprintf("%s: series[%q].color must be a hex colour like #EC4899", prefix, col))
		}
	}

	// Dimensional chart: uses dimension + metrics from a semantic model
	// instead of x/y/sql.
	if w.Dimension != "" || len(w.MetricRefs) > 0 || len(w.Dimensions) > 0 || len(w.Filters) > 0 || len(w.Segments) > 0 || len(w.Sort) > 0 || w.Model != "" {
		if job, handled, err := d.ResolveWidgetSemanticJob(w); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
			return errs
		} else if handled {
			if err := validateSemanticJob(job); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", prefix, err))
			}
			return errs
		}

		errs = append(errs, fmt.Sprintf("%s: dimensional charts require a semantic model; set model on the widget or dashboard", prefix))
		return errs
	}

	switch w.Chart {
	case "pie", "funnel", "treemap":
		// label + value columns
		if w.Label == "" {
			errs = append(errs, fmt.Sprintf("%s: label is required for %s charts", prefix, w.Chart))
		}
		if w.ValueField() == "" {
			errs = append(errs, fmt.Sprintf("%s: value is required for %s charts", prefix, w.Chart))
		}

	case "gauge":
		// value column (current); target is optional
		if w.ValueField() == "" {
			errs = append(errs, fmt.Sprintf("%s: value is required for gauge charts", prefix))
		}

	case "candlestick":
		// x + open/high/low/close
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x is required for candlestick charts", prefix))
		}
		if w.Open == "" {
			errs = append(errs, fmt.Sprintf("%s: open is required for candlestick charts", prefix))
		}
		if w.High == "" {
			errs = append(errs, fmt.Sprintf("%s: high is required for candlestick charts", prefix))
		}
		if w.Low == "" {
			errs = append(errs, fmt.Sprintf("%s: low is required for candlestick charts", prefix))
		}
		if w.Close == "" {
			errs = append(errs, fmt.Sprintf("%s: close is required for candlestick charts", prefix))
		}

	case "sankey":
		// source + target + value columns
		if w.Source == "" {
			errs = append(errs, fmt.Sprintf("%s: source is required for sankey charts", prefix))
		}
		if w.Target == "" {
			errs = append(errs, fmt.Sprintf("%s: target is required for sankey charts", prefix))
		}
		if w.ValueField() == "" {
			errs = append(errs, fmt.Sprintf("%s: value is required for sankey charts", prefix))
		}

	case "heatmap":
		// x + y + value
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x is required for heatmap charts", prefix))
		}
		if len(w.YFields()) == 0 {
			errs = append(errs, fmt.Sprintf("%s: y is required for heatmap charts", prefix))
		}
		if w.ValueField() == "" {
			errs = append(errs, fmt.Sprintf("%s: value is required for heatmap charts", prefix))
		}

	case "calendar":
		// x (date) + value
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x (date column) is required for calendar charts", prefix))
		}
		if w.ValueField() == "" {
			errs = append(errs, fmt.Sprintf("%s: value is required for calendar charts", prefix))
		}

	case "histogram":
		// x (column to bin)
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x is required for histogram charts", prefix))
		}

	case "bubble":
		// x + y + size
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x is required for bubble charts", prefix))
		}
		if len(w.YFields()) == 0 {
			errs = append(errs, fmt.Sprintf("%s: y is required for bubble charts", prefix))
		}
		if w.Size == "" {
			errs = append(errs, fmt.Sprintf("%s: size is required for bubble charts", prefix))
		}

	default:
		// line, bar, area, scatter, combo, sparkline, waterfall, xmr, dumbbell, boxplot, radar
		// all need x + y
		if w.XField() == "" {
			errs = append(errs, fmt.Sprintf("%s: x is required for %s charts", prefix, w.Chart))
		}
		if len(w.YFields()) == 0 {
			errs = append(errs, fmt.Sprintf("%s: y is required for %s charts", prefix, w.Chart))
		}
	}

	if w.Color != nil {
		if w.Color.Field == "" {
			errs = append(errs, fmt.Sprintf("%s: color.field is required", prefix))
		}
		if w.Chart != "bar" && w.Chart != "line" && w.Chart != "area" {
			errs = append(errs, fmt.Sprintf("%s: color is only valid on bar, line, or area charts", prefix))
		}
		if len(w.YFields()) > 1 {
			errs = append(errs, fmt.Sprintf("%s: color with multiple y fields is not supported", prefix))
		}
	}
	if w.Stacked && w.Chart != "bar" {
		errs = append(errs, fmt.Sprintf("%s: stacked is only valid on bar charts", prefix))
	}
	if w.Stacked && w.Color == nil {
		errs = append(errs, fmt.Sprintf("%s: stacked requires color — return one row per category (long format) and set color: { field: <category column> }", prefix))
	}
	if w.Normalized && !w.Stacked {
		errs = append(errs, fmt.Sprintf("%s: normalized requires stacked: true", prefix))
	}
	if w.Normalized && w.Chart != "bar" {
		errs = append(errs, fmt.Sprintf("%s: normalized is only valid on bar charts", prefix))
	}
	if w.Horizontal != nil && *w.Horizontal && w.Chart != "bar" && w.Chart != "funnel" && w.Chart != "forest" {
		errs = append(errs, fmt.Sprintf("%s: horizontal is only valid on bar, funnel and forest charts", prefix))
	}

	return errs
}

func validateSemanticNamedQuery(d *Dashboard, q Query) error {
	model, _, err := d.ResolveSemanticModel(q.Model)
	if err != nil {
		return err
	}
	if model == nil {
		return fmt.Errorf("semantic model is required")
	}
	return validateSemanticJob(&SemanticJob{
		Model:      model,
		Connection: q.Connection,
		Query:      semanticQueryFromNamedQuery(q),
		Models:     d.semanticModels,
	})
}

func validateSemanticJob(job *SemanticJob) error {
	engine, err := sem.NewEngineWithModels(job.Model, job.Models)
	if err != nil {
		return err
	}
	_, err = engine.GenerateSQL(&job.Query)
	return err
}

var validFormatOps = map[string]bool{
	CondIsEmpty: true, CondIsNotEmpty: true,
	CondTextContains: true, CondTextNotContains: true, CondTextStartsWith: true,
	CondTextEndsWith: true, CondTextIsExactly: true,
	CondDateIs: true, CondDateBefore: true, CondDateAfter: true,
	CondGreaterThan: true, CondGreaterThanOrEqual: true, CondLessThan: true,
	CondLessThanOrEqual: true, CondIsEqualTo: true, CondIsNotEqualTo: true,
	CondIsBetween: true, CondIsNotBetween: true,
}

func validateTableColumns(prefix string, w *Widget, errs *[]string) {
	names := make(map[string]bool, len(w.Columns))
	for _, c := range w.Columns {
		names[c.Name] = true
	}
	for _, c := range w.Columns {
		cp := fmt.Sprintf("%s: column %q", prefix, c.Name)
		if c.Like != "" {
			if c.Like == c.Name {
				*errs = append(*errs, cp+".like: cannot reference itself")
			} else if !names[c.Like] {
				*errs = append(*errs, fmt.Sprintf("%s.like: references unknown column %q", cp, c.Like))
			}
		}
		if c.Align != "" && c.Align != "left" && c.Align != "center" && c.Align != "right" {
			*errs = append(*errs, fmt.Sprintf("%s.align: must be left, center, or right", cp))
		}
		for i, layer := range c.Format {
			validateFormatLayer(fmt.Sprintf("%s.format[%d]", cp, i), layer, errs)
		}
	}
}

// validateFormatLayer checks one entry of a column's `format` list. A layer is
// either a condition (`if` set) or a base gradient/flat fill (no `if`).
func validateFormatLayer(prefix string, l FormatLayer, errs *[]string) {
	if l.If != "" {
		if !validFormatOps[l.If] {
			*errs = append(*errs, fmt.Sprintf("%s.if: unknown operator %q", prefix, l.If))
		} else {
			switch l.If {
			case CondIsEmpty, CondIsNotEmpty:
				// No value expected.
			case CondIsBetween, CondIsNotBetween:
				if !isPairValue(l.Value) {
					*errs = append(*errs, fmt.Sprintf("%s: %q requires a two-element value list [low, high]", prefix, l.If))
				}
			default:
				if l.Value == nil {
					*errs = append(*errs, fmt.Sprintf("%s: %q requires a value", prefix, l.If))
				}
			}
		}
	}

	// A gradient is an array backgroundColor (needs >= 2 colors); a string is a flat/single fill.
	colors, isGradient := l.BackgroundColor.([]interface{})
	if isGradient && len(colors) < 2 {
		*errs = append(*errs, fmt.Sprintf("%s.backgroundColor: a gradient needs at least 2 colors (use a single string for a flat fill)", prefix))
	}

	// range/unit describe a gradient's anchors and only apply to a gradient.
	if len(l.Range) > 0 || l.Unit != "" {
		if !isGradient {
			*errs = append(*errs, fmt.Sprintf("%s.range: requires a gradient (a backgroundColor list)", prefix))
		} else {
			if l.Unit != "" && l.Unit != "absolute" && l.Unit != "percent" && l.Unit != "percentile" {
				*errs = append(*errs, fmt.Sprintf("%s.unit: unknown unit %q (expected absolute, percent, or percentile)", prefix, l.Unit))
			}
			if len(l.Range) > 0 && len(l.Range) != len(colors) {
				*errs = append(*errs, fmt.Sprintf("%s.range: has %d anchors but the gradient has %d colors (must match)", prefix, len(l.Range), len(colors)))
			}
			if l.Unit == "percent" || l.Unit == "percentile" {
				for _, v := range l.Range {
					if v < 0 || v > 100 {
						*errs = append(*errs, fmt.Sprintf("%s.range: %s anchors must be between 0 and 100", prefix, l.Unit))
						break
					}
				}
			}
		}
	}

	if !l.Bold && !l.Italic && !l.Underline && !l.Strikethrough && l.TextColor == "" && l.BackgroundColor == nil {
		*errs = append(*errs, fmt.Sprintf("%s: a layer needs at least one style (backgroundColor, textColor, bold, italic, underline, strikethrough)", prefix))
	}
}

func isPairValue(v any) bool {
	arr, ok := v.([]interface{})
	return ok && len(arr) == 2
}
