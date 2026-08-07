package metabase

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	sem "github.com/bruin-data/bruin/semantic-engine"
	semschemas "github.com/bruin-data/bruin/semantic-engine/schemas"
	"github.com/bruin-data/dac/pkg/dashboard"
)

type Project struct {
	Dashboard      *dashboard.Dashboard
	SemanticModels []SemanticModelFile
}

type SemanticModelFile struct {
	Filename string
	Model    *sem.Model
}

type semanticInventory struct {
	Files    []SemanticModelFile
	Models   map[string]*semanticModelInfo
	Metrics  map[string]semanticMetricInfo
	Warnings []string
}

type semanticModelInfo struct {
	ID         string
	Name       string
	Model      *sem.Model
	FieldNames map[string]string
	FieldTypes map[string]string
}

type semanticMetricInfo struct {
	ID        string
	Name      string
	ModelID   string
	ModelName string
}

func collectSemanticInventory(root, dashboardRoot map[string]any, modelSQL map[string]string) semanticInventory {
	inventory := semanticInventory{
		Models:  map[string]*semanticModelInfo{},
		Metrics: map[string]semanticMetricInfo{},
	}

	seenModelNames := map[string]bool{}
	for _, card := range collectSemanticModelCards(root, dashboardRoot) {
		id := firstString(card, "id")
		if id == "" {
			continue
		}
		sql := modelSQL[id]
		if strings.TrimSpace(sql) == "" {
			inventory.Warnings = append(inventory.Warnings, fmt.Sprintf("Metabase model %q was not imported into DAC semantic models because its source SQL was unavailable", cardDisplayName(card, id)))
			continue
		}

		modelName := uniqueSemanticName(normalizeName(cardDisplayName(card, id)), seenModelNames)
		model := &sem.Model{
			Schema:      semschemas.SemanticModelV1ID,
			Name:        modelName,
			Label:       cardDisplayName(card, id),
			Description: firstString(card, "description"),
			Source: sem.Source{
				Table: "(\n" + trimSQLTerminator(sql) + "\n) AS " + quoteIdent(modelName),
			},
		}
		info := &semanticModelInfo{
			ID:         id,
			Name:       modelName,
			Model:      model,
			FieldNames: map[string]string{},
			FieldTypes: map[string]string{},
		}

		seenFieldNames := map[string]bool{}
		for i, col := range resultColumns(card) {
			baseName := normalizeName(col.Name)
			if baseName == "" {
				baseName = fmt.Sprintf("dimension_%d", i+1)
			}
			dimName := uniqueSemanticName(baseName, seenFieldNames)
			dimType := semanticDimensionType(col.Type)
			dim := sem.Dimension{
				Name:       dimName,
				Label:      titleFromName(col.Name),
				Type:       dimType,
				Expression: quoteIdent(col.Name),
			}
			if dimType == "time" {
				dim.Granularities = semanticTimeGranularities(col.Name)
			}
			model.Dimensions = append(model.Dimensions, dim)
			info.FieldNames[col.Name] = dimName
			info.FieldNames[normalizeName(col.Name)] = dimName
			info.FieldTypes[dimName] = dimType
		}

		inventory.Models[id] = info
		inventory.Files = append(inventory.Files, SemanticModelFile{
			Filename: modelName + ".yml",
			Model:    model,
		})
	}

	for _, card := range collectSemanticMetricCards(root, dashboardRoot) {
		id := firstString(card, "id")
		if id == "" {
			continue
		}
		stage, dataset, ok := firstQueryStage(card)
		if !ok {
			inventory.Warnings = append(inventory.Warnings, fmt.Sprintf("Metabase metric %q was not imported because its query stage was unavailable", cardDisplayName(card, id)))
			continue
		}
		sourceID := sourceCardID(stage)
		if sourceID == "" {
			sourceID = sourceCardID(dataset)
		}
		modelInfo := inventory.Models[sourceID]
		if modelInfo == nil {
			inventory.Warnings = append(inventory.Warnings, fmt.Sprintf("Metabase metric %q was not imported because it is not based on an imported Metabase model", cardDisplayName(card, id)))
			continue
		}
		expr, ok := semanticMetricExpression(stage["aggregation"])
		if !ok {
			inventory.Warnings = append(inventory.Warnings, fmt.Sprintf("Metabase metric %q was not imported because its aggregation is unsupported", cardDisplayName(card, id)))
			continue
		}
		filterClauses, ok := mbqlFilterClauses(stage["filter"])
		if !ok {
			inventory.Warnings = append(inventory.Warnings, fmt.Sprintf("Metabase metric %q was not imported because its filter is unsupported", cardDisplayName(card, id)))
			continue
		}
		seenNames := semanticModelUsedNames(modelInfo.Model)
		metricName := uniqueSemanticName(normalizeName(cardDisplayName(card, id)), seenNames)
		modelInfo.Model.Metrics = append(modelInfo.Model.Metrics, sem.Metric{
			Name:        metricName,
			Label:       cardDisplayName(card, id),
			Description: firstString(card, "description"),
			Expression:  expr,
			Filter:      strings.Join(filterClauses, " AND "),
		})
		inventory.Metrics[id] = semanticMetricInfo{
			ID:        id,
			Name:      metricName,
			ModelID:   sourceID,
			ModelName: modelInfo.Name,
		}
	}

	sort.Slice(inventory.Files, func(i, j int) bool {
		return inventory.Files[i].Filename < inventory.Files[j].Filename
	})
	return inventory
}

func semanticWidgetForCard(cardRoot, card map[string]any, name, display string, _ map[string]any, ctx conversionContext) (dashboard.Widget, []string, bool, bool, string, error) {
	stage, dataset, ok := firstQueryStage(card)
	if !ok {
		return dashboard.Widget{}, nil, false, false, "", nil
	}
	sourceID := sourceCardID(stage)
	if sourceID == "" {
		sourceID = sourceCardID(dataset)
	}
	if sourceID == "" {
		return dashboard.Widget{}, nil, false, false, "", nil
	}

	modelInfo := ctx.semanticModels[sourceID]
	if modelInfo == nil {
		return dashboard.Widget{}, nil, false, true, fmt.Sprintf("source card %s is not an explicit imported Metabase model", sourceID), nil
	}

	breakouts, ok, reason := semanticDimensionRefs(stage["breakout"], modelInfo)
	if !ok {
		return dashboard.Widget{}, nil, false, true, reason, nil
	}
	metrics, ok, reason := semanticMetricRefs(stage["aggregation"], modelInfo, ctx)
	if !ok {
		return dashboard.Widget{}, nil, false, true, reason, nil
	}
	filters, ok, reason := semanticFiltersForCard(cardRoot, stage, modelInfo, ctx)
	if !ok {
		return dashboard.Widget{}, nil, false, true, reason, nil
	}

	chart := chartType(display)
	limit := intField(stage, 0, "limit")
	switch display {
	case "scalar", "smartscalar":
		if len(metrics) != 1 {
			return dashboard.Widget{}, nil, false, true, "scalar cards require exactly one explicit Metabase metric", nil
		}
		return dashboard.Widget{
			Name:      name,
			Type:      dashboard.WidgetTypeMetric,
			Col:       defaultWidthForDisplay(display),
			Model:     modelInfo.Name,
			MetricRef: metrics[0],
			Filters:   filters,
			Value:     &dashboard.ValueEncoding{Field: metrics[0], Type: "number"},
		}, nil, true, true, "", nil

	case "line", "area", "bar", "row", "combo":
		if len(breakouts) != 1 {
			return dashboard.Widget{}, nil, false, true, fmt.Sprintf("%s charts require exactly one semantic dimension", chart), nil
		}
		if len(metrics) == 0 {
			return dashboard.Widget{}, nil, false, true, fmt.Sprintf("%s charts require at least one explicit Metabase metric", chart), nil
		}
		widget := dashboard.Widget{
			Name:        name,
			Type:        dashboard.WidgetTypeChart,
			Chart:       chart,
			Col:         defaultWidthForDisplay(display),
			Model:       modelInfo.Name,
			Dimension:   breakouts[0].Name,
			Granularity: breakouts[0].Granularity,
			MetricRefs:  metrics,
			Filters:     filters,
			Sort:        semanticSort(display, breakouts[0].Name, metrics),
			Limit:       limit,
		}
		if display == "row" {
			t := true
			widget.Horizontal = &t
		}
		if display == "combo" && len(metrics) > 1 {
			widget.Lines = []string{metrics[len(metrics)-1]}
		}
		return widget, nil, true, true, "", nil

	case "pie", "funnel", "treemap":
		if len(breakouts) != 1 {
			return dashboard.Widget{}, nil, false, true, fmt.Sprintf("%s charts require exactly one semantic dimension", display), nil
		}
		if len(metrics) != 1 {
			return dashboard.Widget{}, nil, false, true, fmt.Sprintf("%s charts require exactly one explicit Metabase metric", display), nil
		}
		return dashboard.Widget{
			Name:        name,
			Type:        dashboard.WidgetTypeChart,
			Chart:       display,
			Col:         defaultWidthForDisplay(display),
			Model:       modelInfo.Name,
			Dimension:   breakouts[0].Name,
			Granularity: breakouts[0].Granularity,
			MetricRefs:  metrics,
			Filters:     filters,
			Sort:        semanticSort(display, breakouts[0].Name, metrics),
			Limit:       limit,
			Label:       breakouts[0].Name,
			Value:       &dashboard.ValueEncoding{Field: metrics[0], Type: "number"},
		}, nil, true, true, "", nil

	case "table", "object", "pivot", "map":
		if len(metrics) == 0 {
			return dashboard.Widget{}, nil, false, true, "detail-row tables are left SQL-backed because DAC semantic queries aggregate by dimensions", nil
		}
		widget := dashboard.Widget{
			Name:       name,
			Type:       dashboard.WidgetTypeTable,
			Col:        12,
			Model:      modelInfo.Name,
			Dimensions: breakouts,
			MetricRefs: metrics,
			Filters:    filters,
			Limit:      limit,
		}
		widget.Columns = semanticTableColumns(widget.Dimensions, widget.MetricRefs, modelInfo)
		return widget, nil, true, true, "", nil

	default:
		if display == "" {
			return dashboard.Widget{}, nil, false, true, "missing Metabase display type", nil
		}
		return dashboard.Widget{}, nil, false, true, fmt.Sprintf("display type %q is not supported by semantic import", display), nil
	}
}

func firstQueryStage(card map[string]any) (map[string]any, map[string]any, bool) {
	dataset, ok := mapField(card, "dataset_query", "dataset-query")
	if !ok {
		return nil, nil, false
	}
	stages := mapList(dataset, "stages")
	if len(stages) == 0 {
		if query, ok := mapField(dataset, "query"); ok {
			stages = []map[string]any{query}
		}
	}
	if len(stages) == 0 {
		return nil, dataset, false
	}
	return stages[0], dataset, true
}

func semanticDimensionRefs(raw any, modelInfo *semanticModelInfo) ([]dashboard.SemanticDimensionRef, bool, string) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, true, ""
	}
	out := make([]dashboard.SemanticDimensionRef, 0, len(list))
	for _, item := range list {
		name := fieldRefName(item)
		if name == "" {
			return nil, false, "uses a breakout that is not a field reference"
		}
		dim := modelInfo.FieldNames[name]
		if dim == "" {
			dim = modelInfo.FieldNames[normalizeName(name)]
		}
		if dim == "" {
			return nil, false, fmt.Sprintf("breakout field %q is not present on Metabase model %q", name, modelInfo.Name)
		}
		ref := dashboard.SemanticDimensionRef{Name: dim}
		if modelInfo.FieldTypes[dim] == "time" {
			ref.Granularity = metabaseTemporalGranularity(item)
		}
		out = append(out, ref)
	}
	return out, true, ""
}

func semanticMetricRefs(raw any, modelInfo *semanticModelInfo, ctx conversionContext) ([]string, bool, string) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, true, ""
	}
	metrics := make([]string, 0, len(list))
	for _, item := range list {
		id := metricAggregationID(item)
		if id == "" {
			if aggregationLooksSupported(item) {
				return nil, false, "uses unnamed Metabase aggregations; leaving SQL-backed instead of inventing DAC metric names"
			}
			return nil, false, "uses unsupported Metabase aggregations"
		}
		metric := ctx.semanticMetrics[id]
		if metric.Name == "" {
			return nil, false, fmt.Sprintf("references Metabase metric %s, but that metric was not imported", id)
		}
		if metric.ModelID != modelInfo.ID {
			return nil, false, fmt.Sprintf("references Metabase metric %s from a different Metabase model", id)
		}
		metrics = append(metrics, metric.Name)
	}
	return metrics, true, ""
}

func semanticFiltersForCard(cardRoot, stage map[string]any, modelInfo *semanticModelInfo, ctx conversionContext) ([]dashboard.SemanticQueryFilter, bool, string) {
	stageFilters, ok, reason := mbqlSemanticFilters(stage["filter"], modelInfo)
	if !ok {
		return nil, false, reason
	}
	dashboardFilters, ok, reason := dashboardSemanticFilters(cardRoot, modelInfo, ctx)
	if !ok {
		return nil, false, reason
	}
	return append(stageFilters, dashboardFilters...), true, ""
}

func mbqlSemanticFilters(raw any, modelInfo *semanticModelInfo) ([]dashboard.SemanticQueryFilter, bool, string) {
	if raw == nil {
		return nil, true, ""
	}
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return nil, false, "uses unsupported MBQL filters"
	}
	op, _ := parts[0].(string)
	if op == "and" {
		var filters []dashboard.SemanticQueryFilter
		for _, child := range parts[1:] {
			childFilters, ok, reason := mbqlSemanticFilters(child, modelInfo)
			if !ok {
				return nil, false, reason
			}
			filters = append(filters, childFilters...)
		}
		return filters, true, ""
	}
	if op == "is-null" || op == "not-null" {
		if len(parts) < 2 {
			return nil, false, "uses malformed MBQL null filters"
		}
		dim, ok := semanticDimensionName(parts[1], modelInfo)
		if !ok {
			return nil, false, "uses an MBQL filter field that is not present on the Metabase model"
		}
		operator := "is_null"
		if op == "not-null" {
			operator = "is_not_null"
		}
		return []dashboard.SemanticQueryFilter{{Dimension: dim, Operator: operator}}, true, ""
	}
	if len(parts) < 3 {
		return nil, false, "uses malformed MBQL filters"
	}
	dim, ok := semanticDimensionName(parts[1], modelInfo)
	if !ok {
		return nil, false, "uses an MBQL filter field that is not present on the Metabase model"
	}
	operator := mapSemanticFilterOperator(op)
	if operator == "" {
		return nil, false, fmt.Sprintf("uses unsupported MBQL filter operator %q", op)
	}
	return []dashboard.SemanticQueryFilter{{
		Dimension: dim,
		Operator:  operator,
		Value:     parts[2],
	}}, true, ""
}

func dashboardSemanticFilters(cardRoot map[string]any, modelInfo *semanticModelInfo, ctx conversionContext) ([]dashboard.SemanticQueryFilter, bool, string) {
	var filters []dashboard.SemanticQueryFilter
	for _, mapping := range mapList(cardRoot, "parameter_mappings", "parameter-mappings") {
		if !parameterMappingAppliesToCard(mapping, cardRoot) {
			continue
		}
		filter, ok := filterForParameterMapping(mapping, ctx)
		if !ok {
			continue
		}
		field := targetFieldName(mapping["target"])
		if field == "" {
			return nil, false, fmt.Sprintf("dashboard filter %q is mapped to an unsupported target", filter.Name)
		}
		dim := modelInfo.FieldNames[field]
		if dim == "" {
			dim = modelInfo.FieldNames[normalizeName(field)]
		}
		if dim == "" {
			return nil, false, fmt.Sprintf("dashboard filter %q targets field %q, which is not present on Metabase model %q", filter.Name, field, modelInfo.Name)
		}
		filters = append(filters, semanticFilterFromDashboardFilter(dim, filter))
	}
	return filters, true, ""
}

func semanticFilterFromDashboardFilter(dim string, filter dashboard.Filter) dashboard.SemanticQueryFilter {
	name := filter.Name
	switch filter.Type {
	case "date-range":
		return dashboard.SemanticQueryFilter{
			Dimension: dim,
			Operator:  "between",
			Value: map[string]interface{}{
				"start": "{{ filters." + name + ".start }}",
				"end":   "{{ filters." + name + ".end }}",
			},
		}
	case "date":
		return dashboard.SemanticQueryFilter{Dimension: dim, Operator: "equals", Value: "{{ filters." + name + " }}"}
	case "number":
		return dashboard.SemanticQueryFilter{Dimension: dim, Operator: "equals", Value: "{{ filters." + name + " }}"}
	case "select", "text":
		if filter.Multiple {
			return dashboard.SemanticQueryFilter{
				Expression: "{" + dim + "} IN ('{{ filters." + name + " | join(\"','\") }}')",
			}
		}
		return dashboard.SemanticQueryFilter{Dimension: dim, Operator: "equals", Value: "{{ filters." + name + " }}"}
	default:
		return dashboard.SemanticQueryFilter{Dimension: dim, Operator: "equals", Value: "{{ filters." + name + " }}"}
	}
}

func semanticMetricExpression(raw any) (string, bool) {
	list, ok := raw.([]any)
	if !ok || len(list) != 1 {
		return "", false
	}
	parts, ok := list[0].([]any)
	if !ok || len(parts) == 0 {
		return "", false
	}
	fn, _ := parts[0].(string)
	field := ""
	for _, part := range parts[1:] {
		if name := fieldRefName(part); name != "" {
			field = name
			break
		}
	}
	switch fn {
	case "count":
		if field == "" {
			return "COUNT(*)", true
		}
		return "COUNT(" + quoteIdent(field) + ")", true
	case "sum", "avg", "min", "max":
		if field == "" {
			return "", false
		}
		return strings.ToUpper(fn) + "(" + quoteIdent(field) + ")", true
	case "distinct", "count-distinct":
		if field == "" {
			return "", false
		}
		return "COUNT(DISTINCT " + quoteIdent(field) + ")", true
	default:
		return "", false
	}
}

func metricAggregationID(raw any) string {
	switch item := raw.(type) {
	case []any:
		if len(item) == 0 {
			return ""
		}
		fn, _ := item[0].(string)
		if fn != "metric" {
			return ""
		}
		for _, part := range item[1:] {
			if id := metricIDValue(part); id != "" {
				return id
			}
		}
	case map[string]any:
		if id := firstString(item, "metric_id", "metric-id", "id"); id != "" {
			return id
		}
	}
	return ""
}

func metricIDValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(strings.TrimPrefix(v, "metric__"))
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case map[string]any:
		return firstString(v, "metric_id", "metric-id", "id")
	default:
		return ""
	}
}

func aggregationLooksSupported(raw any) bool {
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return false
	}
	fn, _ := parts[0].(string)
	switch fn {
	case "count", "sum", "avg", "min", "max", "distinct", "count-distinct":
		return true
	default:
		return false
	}
}

func semanticDimensionName(raw any, modelInfo *semanticModelInfo) (string, bool) {
	name := fieldRefName(raw)
	if name == "" {
		return "", false
	}
	dim := modelInfo.FieldNames[name]
	if dim == "" {
		dim = modelInfo.FieldNames[normalizeName(name)]
	}
	return dim, dim != ""
}

func mapSemanticFilterOperator(op string) string {
	switch op {
	case "=":
		return "equals"
	case "!=":
		return "not_equals"
	case "<":
		return "lt"
	case "<=":
		return "lte"
	case ">":
		return "gt"
	case ">=":
		return "gte"
	default:
		return ""
	}
}

func semanticSort(display, dimension string, metrics []string) []dashboard.SemanticSort {
	if dimension != "" && chronologicalDisplay(display) {
		return []dashboard.SemanticSort{{Name: dimension, Direction: "asc"}}
	}
	if len(metrics) > 0 {
		return []dashboard.SemanticSort{{Name: metrics[0], Direction: "desc"}}
	}
	return nil
}

func semanticTableColumns(dimensions []dashboard.SemanticDimensionRef, metrics []string, modelInfo *semanticModelInfo) []dashboard.TableColumn {
	names := make([]string, 0, len(dimensions)+len(metrics))
	for _, dim := range dimensions {
		names = append(names, dim.Name)
	}
	names = append(names, metrics...)
	if len(names) == 0 {
		for _, dim := range modelInfo.Model.Dimensions {
			names = append(names, dim.Name)
		}
	}
	out := make([]dashboard.TableColumn, 0, len(names))
	for _, name := range names {
		out = append(out, dashboard.TableColumn{Name: name, Label: titleFromName(name)})
	}
	return out
}

func collectSemanticModelCards(root, dashboardRoot map[string]any) []map[string]any {
	var cards []map[string]any
	add := func(card map[string]any) {
		if firstString(card, "type") != "model" {
			return
		}
		cards = append(cards, card)
	}
	collectMetabaseCards(root, dashboardRoot, []string{"x-dac-metabase-source-cards", "source_cards", "source-cards"}, add)
	for _, dashcard := range extractCards(dashboardRoot) {
		if card, ok := mapField(dashcard, "card", "question"); ok {
			add(card)
		}
	}
	return dedupeCards(cards)
}

func collectSemanticMetricCards(root, dashboardRoot map[string]any) []map[string]any {
	var cards []map[string]any
	addAny := func(card map[string]any) {
		cards = append(cards, card)
	}
	collectMetabaseCards(root, dashboardRoot, []string{"x-dac-metabase-metrics", "metric_cards", "metric-cards"}, addAny)

	addTyped := func(card map[string]any) {
		if firstString(card, "type") == "metric" {
			cards = append(cards, card)
		}
	}
	collectMetabaseCards(root, dashboardRoot, []string{"x-dac-metabase-source-cards", "source_cards", "source-cards"}, addTyped)
	for _, dashcard := range extractCards(dashboardRoot) {
		if card, ok := mapField(dashcard, "card", "question"); ok {
			addTyped(card)
		}
	}
	return dedupeCards(cards)
}

func collectMetabaseCards(root, dashboardRoot map[string]any, keys []string, add func(map[string]any)) {
	for _, source := range []map[string]any{root, dashboardRoot} {
		for _, key := range keys {
			if sourceCards, ok := mapField(source, key); ok {
				ids := make([]string, 0, len(sourceCards))
				for id := range sourceCards {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				for _, id := range ids {
					if card, ok := sourceCards[id].(map[string]any); ok {
						add(card)
					}
				}
			}
			for _, card := range mapList(source, key) {
				add(card)
			}
		}
	}
}

func dedupeCards(cards []map[string]any) []map[string]any {
	seen := map[string]bool{}
	out := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		id := firstString(card, "id")
		key := id
		if key == "" {
			key = cardDisplayName(card, "")
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, card)
	}
	return out
}

func cardDisplayName(card map[string]any, fallback string) string {
	name := firstString(card, "name", "display_name", "display-name", "title")
	if name != "" {
		return name
	}
	if fallback != "" {
		return "metabase_" + fallback
	}
	return "metabase"
}

func semanticDimensionType(sourceType string) string {
	switch {
	case isDateType(sourceType):
		return "time"
	case isNumericType(sourceType):
		return "number"
	case strings.Contains(strings.ToLower(sourceType), "bool"):
		return "boolean"
	default:
		return "string"
	}
}

func semanticTimeGranularities(field string) map[string]string {
	expr := quoteIdent(field)
	return map[string]string{
		"day":     "date_trunc('day', " + expr + ")",
		"week":    "date_trunc('week', " + expr + ")",
		"month":   "date_trunc('month', " + expr + ")",
		"quarter": "date_trunc('quarter', " + expr + ")",
		"year":    "date_trunc('year', " + expr + ")",
	}
}

func metabaseTemporalGranularity(raw any) string {
	grain := ""
	var walk func(any)
	walk = func(v any) {
		if grain != "" {
			return
		}
		switch x := v.(type) {
		case map[string]any:
			for _, key := range []string{"temporal-unit", "temporal_unit", "unit"} {
				if s, ok := x[key].(string); ok {
					grain = normalizeTemporalGranularity(s)
					if grain != "" {
						return
					}
				}
			}
			for _, value := range x {
				walk(value)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(raw)
	return grain
}

func normalizeTemporalGranularity(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "day", "date":
		return "day"
	case "week":
		return "week"
	case "month":
		return "month"
	case "quarter":
		return "quarter"
	case "year":
		return "year"
	default:
		return ""
	}
}

func uniqueSemanticName(base string, seen map[string]bool) string {
	if base == "" {
		base = "metabase"
	}
	name := base
	for i := 2; seen[name]; i++ {
		name = fmt.Sprintf("%s_%d", base, i)
	}
	seen[name] = true
	return name
}

func semanticModelUsedNames(model *sem.Model) map[string]bool {
	seen := map[string]bool{}
	for _, dim := range model.Dimensions {
		seen[dim.Name] = true
	}
	for _, metric := range model.Metrics {
		seen[metric.Name] = true
	}
	for _, segment := range model.Segments {
		seen[segment.Name] = true
	}
	return seen
}

func attachSemanticModels(d *dashboard.Dashboard, files []SemanticModelFile) {
	if len(files) == 0 {
		return
	}
	models := make(map[string]*sem.Model, len(files))
	for _, file := range files {
		if file.Model != nil {
			models[file.Model.Name] = file.Model
		}
	}
	d.SetProjectContext("", models, map[string]error{})
}
