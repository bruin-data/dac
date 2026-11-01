package dashboard

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
	"gopkg.in/yaml.v3"
)

// SemanticFormat is the object alternative to a d3 format string.
// A nil precision uses Intl's defaults; an explicit zero means no decimals.
type SemanticFormat struct {
	Type     string `yaml:"type,omitempty" json:"type,omitempty"`
	Currency string `yaml:"currency,omitempty" json:"currency,omitempty"`
	Decimals *int   `yaml:"decimals,omitempty" json:"decimals,omitempty"`
}

func emptyFormat(value any) bool { return value == nil || value == "" }

// formatOrNil keeps omitempty working now that formats are `any`: an empty
// string (e.g. from the Metabase importer) would otherwise be serialized.
func formatOrNil(value any) any {
	if emptyFormat(value) {
		return nil
	}
	return value
}

// decodeFormat keeps a YAML scalar as its source text, as the old string fields
// did, so unquoted d3 specs such as `.2` or `05` don't become numbers.
func decodeFormat(node *yaml.Node) (any, error) {
	for node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	switch {
	case node.Kind == 0 || node.Tag == "!!null":
		return nil, nil
	case node.Kind == yaml.ScalarNode:
		return formatOrNil(node.Value), nil
	}
	var value any
	err := node.Decode(&value)
	return value, err
}

// ParseDisplayFormat also resolves per-field defaults on multi-series axes.
func ParseDisplayFormat(value any, field string) (string, *SemanticFormat) {
	if spec, ok := value.(string); ok {
		return spec, nil
	}
	if value == nil {
		return "", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", nil
	}
	var format struct {
		SemanticFormat
		Fields map[string]SemanticFormat `json:"fields"`
	}
	if json.Unmarshal(data, &format) != nil {
		return "", nil
	}
	if format.Fields != nil {
		for name, f := range format.Fields {
			if strings.EqualFold(name, field) {
				return "", &f
			}
		}
		return "", nil
	}
	return "", &format.SemanticFormat
}

// fieldFormat narrows a multi-series format to the given field.
func fieldFormat(value any, field string) any {
	spec, format := ParseDisplayFormat(value, field)
	if format != nil {
		return *format
	}
	return formatOrNil(spec)
}

// Validate only the new format objects; legacy strings retain their existing checks.
func validateDisplayFormat(value any) error {
	if emptyFormat(value) {
		return nil
	}
	if _, legacy := value.(string); legacy {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return fmt.Errorf("format must be a string or an object")
	}
	if len(object) == 0 {
		return fmt.Errorf("format must specify type, currency, or decimals")
	}
	for key, raw := range object {
		switch key {
		case "type":
			var typ string
			if json.Unmarshal(raw, &typ) != nil || !slices.Contains([]string{"number", "decimal", "currency", "percentage"}, typ) {
				return fmt.Errorf("unsupported format type %s", raw)
			}
		case "currency":
			var code string
			if err := json.Unmarshal(raw, &code); err != nil {
				return err
			}
			// Shape only, like the schema and Intl: Go's ISO table lags newer codes (VES, MRU, SLE).
			if len(code) != 3 || strings.Trim(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
				return fmt.Errorf("unsupported currency code %q", code)
			}
		case "decimals":
			var decimals int
			if string(raw) == "null" || json.Unmarshal(raw, &decimals) != nil || decimals < 0 || decimals > 100 {
				return fmt.Errorf("decimals must be an integer between 0 and 100")
			}
		case "fields":
			var fields map[string]json.RawMessage
			if len(object) != 1 || json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
				return fmt.Errorf("fields must be a non-empty map used without other format properties")
			}
			for name, field := range fields {
				var child map[string]json.RawMessage
				if json.Unmarshal(field, &child) != nil || child == nil || child["fields"] != nil {
					return fmt.Errorf("field %q must specify a numeric format object", name)
				}
				if err := validateDisplayFormat(field); err != nil {
					return fmt.Errorf("field %q: %w", name, err)
				}
			}
		default:
			return fmt.Errorf("unknown format property %q", key)
		}
	}
	return nil
}

// Model currency codes are case-insensitive, like Intl: the semantic engine
// never validated them, so models written with "usd" must keep loading.
func modelFormat(f *sem.Format) SemanticFormat {
	return SemanticFormat{Type: f.Type, Currency: strings.ToUpper(f.Currency)}
}

// Put inherited defaults in the existing encoding/column formats. Explicit
// widget formats win; no extra widget or query-result metadata is needed.
func (d *Dashboard) inheritWidgetFormats(w *Widget) {
	job, handled, err := d.ResolveWidgetSemanticJob(w)
	if err != nil || !handled || w.HasInlineData() {
		return
	}
	// Keyed by lowercase name: warehouses such as Snowflake return uppercase aliases.
	formats := map[string]SemanticFormat{}
	for _, metric := range job.Model.Metrics {
		if metric.Format == nil {
			continue
		}
		format := modelFormat(metric.Format)
		// The model's decimals is a plain int, so zero means "unset" (Intl defaults).
		if metric.Format.Decimals != 0 {
			decimals := metric.Format.Decimals
			format.Decimals = &decimals
		}
		// Skip invalid model formats instead of failing: the semantic schema accepts
		// any currency string, and models were never validated for display.
		if validateDisplayFormat(format) == nil {
			formats[strings.ToLower(metric.Name)] = format
		}
	}
	resolve := func(fields []string) any {
		selected := map[string]SemanticFormat{}
		for _, field := range fields {
			if f, ok := formats[strings.ToLower(field)]; ok {
				selected[field] = f
			}
		}
		if len(selected) == 0 {
			return nil
		}
		if len(fields) == 1 {
			return selected[fields[0]]
		}
		return map[string]any{"fields": selected}
	}
	// Vega-Lite specs name their own fields, so a chart without `dimension:` has no y;
	// add one when it carries model formats, since the frontend reads them from y.
	if w.Chart == "vega-lite" && w.Y == nil {
		if format := resolve(job.Query.Metrics); format != nil {
			w.Y = &AxisEncoding{Field: append([]string(nil), job.Query.Metrics...), Format: format}
		}
	}
	for _, axis := range []*AxisEncoding{w.X, w.Y, w.Y2} {
		if axis == nil || axis.Type == "date" || axis.Type == "category" {
			continue
		}
		if emptyFormat(axis.Format) {
			axis.Format = resolve(axis.FieldList())
		} else {
			axis.Format = mergeFieldFormats(axis.Format, axis.FieldList(), formats)
		}
	}
	if w.Value == nil && len(job.Query.Metrics) > 0 && (w.Type == WidgetTypeMetric || slices.Contains([]string{"pie", "funnel", "treemap", "gauge", "sankey", "calendar", "heatmap"}, w.Chart)) {
		// Follow the frontend's value fallback (y before the first metric), and only
		// add a value encoding when it carries a format, so unformatted widgets load unchanged.
		value := &ValueEncoding{Field: job.Query.Metrics[0]}
		value.Format = resolve([]string{value.Field})
		// A heatmap's y is a category dimension, never the cell value.
		if ys := w.YFields(); len(ys) > 0 && w.Chart != "heatmap" {
			// y's format (explicit, or inherited above) also covers the value it supplies.
			value = &ValueEncoding{Field: ys[0], Type: w.Y.Type, Format: fieldFormat(w.Y.Format, ys[0])}
		}
		if value.Format != nil {
			w.Value, w.inheritedValue = value, true
		}
	}
	if w.Value != nil && emptyFormat(w.Value.Format) && w.Value.Type != "date" && w.Value.Type != "category" {
		w.Value.Format = resolve([]string{w.Value.Field})
	}
	// Without model formats there is nothing to inherit; leave the columns as written.
	if len(formats) == 0 || (w.Type != WidgetTypeTable && w.Type != WidgetTypePivotTable) {
		return
	}
	automaticColumns := len(w.Columns) == 0
	if automaticColumns {
		for _, dim := range job.Query.Dimensions {
			w.Columns = append(w.Columns, TableColumn{Name: dim.Name})
		}
	}
	if automaticColumns || w.Type == WidgetTypePivotTable {
		for _, field := range job.Query.Metrics {
			if !slices.ContainsFunc(w.Columns, func(col TableColumn) bool { return col.Name == field }) {
				w.Columns = append(w.Columns, TableColumn{Name: field})
			}
		}
	}
	for i := range w.Columns {
		col := &w.Columns[i]
		if format, ok := formats[strings.ToLower(col.Name)]; ok {
			if emptyFormat(col.Number) {
				col.Number = format
			} else if col.Number == "currency" && format.Currency != "" {
				format.Type = "currency"
				col.Number = format
			}
		}
	}
}

// mergeFieldFormats adds model defaults to a partial `fields` map, so it only
// overrides the fields it lists. Other formats, and invalid ones (left for
// validation to report), are returned unchanged.
func mergeFieldFormats(value any, fields []string, defaults map[string]SemanticFormat) any {
	if validateDisplayFormat(value) != nil {
		return value
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var format struct {
		Fields map[string]SemanticFormat `json:"fields"`
	}
	if json.Unmarshal(data, &format) != nil || format.Fields == nil {
		return value
	}
	merged := false
	for _, field := range fields {
		if _, explicit := ParseDisplayFormat(value, field); explicit != nil {
			continue
		}
		if f, ok := defaults[strings.ToLower(field)]; ok {
			format.Fields[field] = f
			merged = true
		}
	}
	if !merged {
		return value
	}
	return map[string]any{"fields": format.Fields}
}

func validateWidgetFormats(prefix string, w *Widget) []string {
	var errs []string
	check := func(name, typ string, value any) {
		if err := validateDisplayFormat(value); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s: %v", prefix, name, err))
		}
		if _, isString := value.(string); typ == "date" && value != nil && !isString {
			errs = append(errs, prefix+": "+name+": date formats must be strings")
		}
	}
	axis := func(name string, enc *AxisEncoding) {
		if enc != nil {
			check(name+".format", enc.Type, enc.Format)
		}
	}
	axis("x", w.X)
	axis("y", w.Y)
	axis("y2", w.Y2)
	// An inherited value's format comes from y (checked above) or a valid model format.
	if w.Value != nil && !w.inheritedValue {
		check("value.format", w.Value.Type, w.Value.Format)
	}
	for _, column := range w.Columns {
		check("column "+column.Name+" number", "", column.Number)
		axis("column "+column.Name+" x", column.X)
		axis("column "+column.Name+" y", column.Y)
	}
	return errs
}
