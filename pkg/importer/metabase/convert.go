package metabase

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/schemas"
	"gopkg.in/yaml.v3"
)

type Options struct {
	Connection     string
	Strict         bool
	Semantic       bool
	SemanticStrict bool
}

type Report struct {
	WidgetCount         int
	SQLWidgetCount      int
	SemanticWidgetCount int
	SemanticModelCount  int
	PlaceholderCount    int
	Warnings            []string
}

type positionedWidget struct {
	w      dashboard.Widget
	row    int
	col    int
	sizeX  int
	source string
}

type columnInfo struct {
	Name string
	Type string
}

type metabaseTableInfo struct {
	ID         string
	Name       string
	Schema     string
	Expression string
	FieldNames map[string]string
	Columns    []columnInfo
}

type sqlTemplateVariable struct {
	FilterName string
	UseFilter  bool
	Quote      bool
	Default    any
	HasDefault bool
}

type conversionContext struct {
	filterNames     map[string]string
	filters         map[string]dashboard.Filter
	modelSQL        map[string]string
	tables          map[string]*metabaseTableInfo
	semantic        bool
	semanticStrict  bool
	semanticModels  map[string]*semanticModelInfo
	semanticMetrics map[string]semanticMetricInfo
}

// Convert converts a Metabase dashboard API response or serialized dashboard
// document into a DAC dashboard. Native SQL questions are converted to data
// widgets. Supported query-builder/MBQL cards are compiled to SQL; unsupported
// cards are represented as text placeholders unless Strict is set.
func Convert(data []byte, opts Options) (*dashboard.Dashboard, Report, error) {
	project, report, err := ConvertProject(data, opts)
	if err != nil {
		return nil, report, err
	}
	return project.Dashboard, report, nil
}

// ConvertProject converts a Metabase dashboard and returns any generated DAC
// semantic model files alongside the dashboard.
func ConvertProject(data []byte, opts Options) (*Project, Report, error) {
	root, err := parseInput(data)
	if err != nil {
		return nil, Report{}, err
	}

	dashboardRoot := unwrapDashboard(root)
	name := firstString(dashboardRoot, "name", "display_name", "display-name", "title")
	if name == "" {
		name = "Imported Metabase Dashboard"
	}

	report := Report{}
	dac := &dashboard.Dashboard{
		Schema:      schemas.DashboardV1ID,
		Name:        name,
		Description: firstString(dashboardRoot, "description"),
		Connection:  opts.Connection,
	}
	project := &Project{Dashboard: dac}

	semanticEnabled := opts.Semantic || opts.SemanticStrict
	modelSQL := collectModelSQL(root, dashboardRoot)
	tables := collectMetabaseTables(root, dashboardRoot)
	var inventory semanticInventory
	if semanticEnabled {
		inventory = collectSemanticInventory(root, dashboardRoot, modelSQL)
		project.SemanticModels = inventory.Files
		report.SemanticModelCount = len(project.SemanticModels)
		report.Warnings = append(report.Warnings, inventory.Warnings...)
	}
	ctx := conversionContext{
		filterNames:     map[string]string{},
		filters:         map[string]dashboard.Filter{},
		modelSQL:        modelSQL,
		tables:          tables,
		semantic:        semanticEnabled,
		semanticStrict:  opts.SemanticStrict,
		semanticModels:  inventory.Models,
		semanticMetrics: inventory.Metrics,
	}
	seenFilters := map[string]bool{}
	for _, filter := range convertDashboardFilters(dashboardRoot) {
		dac.Filters = append(dac.Filters, filter)
		ctx.filters[filter.Name] = filter
		seenFilters[filter.Name] = true
		registerFilterAliases(ctx.filterNames, filter.Name, filter.Name)
	}
	for _, param := range mapList(dashboardRoot, "parameters", "params") {
		filterName := normalizeName(firstString(param, "slug", "name", "id"))
		registerFilterAliases(ctx.filterNames, filterName, firstString(param, "id"), firstString(param, "slug"), firstString(param, "name"))
	}

	cards := extractCards(dashboardRoot)
	if len(cards) == 0 {
		return nil, report, fmt.Errorf("no Metabase dashboard cards found")
	}

	positioned := make([]positionedWidget, 0, len(cards))
	for i, cardRoot := range cards {
		w, warnings, tagFilters, convertedSQL, convertedSemantic, placeholder, err := convertCard(cardRoot, i+1, ctx, opts)
		if err != nil {
			return nil, report, err
		}
		for _, filter := range tagFilters {
			if seenFilters[filter.Name] {
				continue
			}
			dac.Filters = append(dac.Filters, filter)
			ctx.filters[filter.Name] = filter
			seenFilters[filter.Name] = true
		}
		report.Warnings = append(report.Warnings, warnings...)
		if convertedSQL {
			report.SQLWidgetCount++
		}
		if convertedSemantic {
			report.SemanticWidgetCount++
		}
		if placeholder {
			report.PlaceholderCount++
		}
		positioned = append(positioned, positionedWidget{
			w:      w,
			row:    intField(cardRoot, 0, "row", "y"),
			col:    intField(cardRoot, 0, "col", "x"),
			sizeX:  intField(cardRoot, defaultWidthForWidget(w), "size_x", "size-x", "width", "w"),
			source: cardName(cardRoot, i+1),
		})
	}

	dac.Rows = layoutRows(positioned)
	report.WidgetCount = len(positioned)
	attachSemanticModels(dac, project.SemanticModels)
	if err := dashboard.Validate(dac); err != nil {
		return nil, report, fmt.Errorf("generated dashboard failed validation: %w", err)
	}
	return project, report, nil
}

func parseInput(data []byte) (map[string]any, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err == nil {
		root, ok := normalizeValue(raw).(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Metabase input must be an object")
		}
		return root, nil
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing Metabase input as JSON or YAML: %w", err)
	}
	root, ok := normalizeValue(raw).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Metabase input must be an object")
	}
	return root, nil
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = normalizeValue(v)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[fmt.Sprint(k)] = normalizeValue(v)
		}
		return out
	case []any:
		for i := range x {
			x[i] = normalizeValue(x[i])
		}
		return x
	default:
		return v
	}
}

func unwrapDashboard(root map[string]any) map[string]any {
	for _, key := range []string{"dashboard", "data"} {
		if child, ok := mapField(root, key); ok && looksLikeDashboard(child) {
			return child
		}
	}
	return root
}

func looksLikeDashboard(m map[string]any) bool {
	if firstString(m, "name", "display_name", "display-name", "title") == "" {
		return false
	}
	return len(extractCards(m)) > 0
}

func extractCards(root map[string]any) []map[string]any {
	for _, key := range []string{"dashcards", "ordered_cards", "ordered-cards", "cards"} {
		if list := mapList(root, key); len(list) > 0 {
			return list
		}
	}
	return nil
}

func convertDashboardFilters(root map[string]any) []dashboard.Filter {
	params := mapList(root, "parameters", "params")
	seen := map[string]bool{}
	var filters []dashboard.Filter
	for _, param := range params {
		name := normalizeName(firstString(param, "slug", "name", "id"))
		if name == "" || seen[name] {
			continue
		}
		filter := dashboard.Filter{
			Name:    name,
			Type:    dacFilterType(firstString(param, "type", "section_id", "section-id")),
			Default: normalizeFilterDefault(firstValue(param, "default", "default_value", "default-value"), firstString(param, "type")),
		}
		if values := extractStaticFilterValues(param); len(values) > 0 {
			filter.Options = &dashboard.FilterOptions{Values: values}
		}
		filters = append(filters, filter)
		seen[name] = true
	}
	return filters
}

func registerFilterAliases(aliases map[string]string, dacName string, rawNames ...string) {
	if aliases == nil {
		return
	}
	if dacName == "" {
		return
	}
	for _, raw := range rawNames {
		if raw == "" {
			continue
		}
		aliases[raw] = dacName
		aliases[normalizeName(raw)] = dacName
	}
}

func mappedFilterName(aliases map[string]string, rawNames ...string) string {
	for _, raw := range rawNames {
		if raw == "" {
			continue
		}
		if aliases != nil {
			if mapped := aliases[raw]; mapped != "" {
				return mapped
			}
			if mapped := aliases[normalizeName(raw)]; mapped != "" {
				return mapped
			}
		}
	}
	return ""
}

func registerTemplateVariableAlias(aliases map[string]sqlTemplateVariable, raw string, info sqlTemplateVariable) {
	if aliases == nil || raw == "" {
		return
	}
	aliases[raw] = info
	aliases[normalizeName(raw)] = info
}

func dashboardTemplateTagFilterAliases(cardRoot map[string]any, ctx conversionContext) map[string]string {
	aliases := map[string]string{}
	for _, mapping := range mapList(cardRoot, "parameter_mappings", "parameter-mappings") {
		if !parameterMappingAppliesToCard(mapping, cardRoot) {
			continue
		}
		filter, ok := filterForParameterMapping(mapping, ctx)
		if !ok {
			continue
		}
		tagName := targetTemplateTagName(mapping["target"])
		if tagName == "" {
			tagName = targetFieldName(mapping["target"])
		}
		if tagName == "" {
			continue
		}
		registerFilterAliases(aliases, filter.Name, tagName)
	}
	return aliases
}

func convertCard(cardRoot map[string]any, index int, ctx conversionContext, opts Options) (dashboard.Widget, []string, []dashboard.Filter, bool, bool, bool, error) {
	card, _ := mapField(cardRoot, "card", "question")
	if card == nil {
		card = cardRoot
	}
	if virtual, ok := mapField(cardRoot, "virtual_card", "virtual-card"); ok {
		card = virtual
	}
	if virtual, ok := mapField(mapFieldOrEmpty(cardRoot, "visualization_settings", "visualization-settings"), "virtual_card", "virtual-card"); ok {
		card = virtual
	}

	name := cardName(cardRoot, index)
	display := strings.ToLower(firstString(card, "display", "display_type", "display-type", "type"))
	settings := mergeMaps(
		mapFieldOrEmpty(card, "visualization_settings", "visualization-settings"),
		mapFieldOrEmpty(cardRoot, "visualization_settings", "visualization-settings"),
	)

	if text := textContent(cardRoot, card, settings); text != "" && !hasNativeSQL(card) {
		return dashboard.Widget{
			Name:    name,
			Type:    dashboard.WidgetTypeText,
			Content: text,
			Col:     12,
		}, nil, nil, false, false, false, nil
	}

	sql, templateWarnings := nativeSQL(card, dashboardTemplateTagFilterAliases(cardRoot, ctx))
	if ctx.semantic && sql == "" {
		widget, warnings, converted, attempted, reason, err := semanticWidgetForCard(cardRoot, card, name, display, settings, ctx)
		if err != nil {
			return dashboard.Widget{}, nil, nil, false, false, false, err
		}
		if converted {
			for i, warning := range warnings {
				warnings[i] = fmt.Sprintf("%s: %s", name, warning)
			}
			return widget, warnings, nil, false, true, false, nil
		}
		if attempted {
			if ctx.semanticStrict {
				return dashboard.Widget{}, nil, nil, false, false, false, fmt.Errorf("%s: cannot convert to semantic widget: %s", name, reason)
			}
			if reason != "" {
				templateWarnings = append(templateWarnings, "semantic import skipped: "+reason)
			}
		}
	}
	if sql == "" {
		var modelWarnings []string
		sql, modelWarnings = modelBackedSQL(card, cardRoot, ctx)
		templateWarnings = append(templateWarnings, modelWarnings...)
	}
	if sql == "" {
		var tableWarnings []string
		var displayOverride string
		sql, tableWarnings, displayOverride = physicalTableBackedSQL(card, cardRoot, ctx)
		templateWarnings = append(templateWarnings, tableWarnings...)
		if displayOverride != "" {
			display = displayOverride
		}
	}
	if sql == "" {
		reason := "Metabase query-builder/MBQL cards cannot be converted because DAC needs native SQL"
		if display == "" {
			reason = "Metabase card does not contain a native SQL query"
		}
		if len(templateWarnings) > 0 {
			reason = templateWarnings[len(templateWarnings)-1]
		}
		if opts.Strict {
			return dashboard.Widget{}, nil, nil, false, false, false, fmt.Errorf("%s: %s", name, reason)
		}
		content := fmt.Sprintf("**Unsupported Metabase card**\n\n%s.\n\nOriginal card: `%s`", reason, name)
		warnings := make([]string, 0, len(templateWarnings)+1)
		for _, warning := range templateWarnings {
			warnings = append(warnings, fmt.Sprintf("%s: %s", name, warning))
		}
		if len(templateWarnings) == 0 || templateWarnings[len(templateWarnings)-1] != reason {
			warnings = append(warnings, fmt.Sprintf("%s: %s", name, reason))
		}
		return dashboard.Widget{
			Name:    name,
			Type:    dashboard.WidgetTypeText,
			Content: content,
			Col:     12,
		}, warnings, nil, false, false, true, nil
	}

	columns := resultColumns(card)
	widget, warnings := widgetForDisplay(name, display, sql, settings, columns)
	warnings = append(warnings, templateWarnings...)
	for i, warning := range warnings {
		warnings[i] = fmt.Sprintf("%s: %s", name, warning)
	}
	return widget, warnings, nil, true, false, false, nil
}

func hasNativeSQL(card map[string]any) bool {
	sql, _ := nativeSQL(card, nil)
	return sql != ""
}

func nativeSQL(card map[string]any, filterNames map[string]string) (string, []string) {
	dataset, ok := mapField(card, "dataset_query", "dataset-query")
	if !ok {
		return "", nil
	}
	query, templateTags := nativeQueryAndTemplateTags(dataset)
	if query == "" {
		return "", nil
	}

	var warnings []string
	variables := map[string]sqlTemplateVariable{}
	for tagName, raw := range templateTags {
		tag, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		dacName := normalizeName(firstString(tag, "name", "display_name", "display-name"))
		if dacName == "" {
			dacName = normalizeName(tagName)
		}
		tagType := firstString(tag, "type", "widget_type", "widget-type")
		filterName := mappedFilterName(filterNames, tagName, dacName, firstString(tag, "name"), firstString(tag, "display_name", "display-name"))
		defaultValue, hasDefault := firstPresentValue(tag, "default", "default_value", "default-value")
		info := sqlTemplateVariable{
			FilterName: filterName,
			UseFilter:  filterName != "",
			Quote:      filterTypeNeedsQuotes(dacFilterType(tagType)),
			Default:    defaultValue,
			HasDefault: hasDefault && defaultValue != nil,
		}
		for _, alias := range []string{tagName, dacName, firstString(tag, "name"), firstString(tag, "display_name", "display-name")} {
			registerTemplateVariableAlias(variables, alias, info)
		}
		if strings.EqualFold(tagType, "dimension") {
			warnings = append(warnings, fmt.Sprintf("field filter %q was converted to a plain DAC filter reference; review the WHERE clause", tagName))
		}
	}

	converted, optionalCount, unresolved := convertSQLTemplate(query, variables)
	if optionalCount > 0 {
		warnings = append(warnings, "Metabase optional SQL clause markers [[...]] were removed; review empty-filter behavior")
	}
	for _, name := range unresolved {
		warnings = append(warnings, fmt.Sprintf("native SQL variable %q is not mapped to a dashboard filter and has no default; review the generated SQL", name))
	}
	return converted, warnings
}

func nativeQueryAndTemplateTags(dataset map[string]any) (string, map[string]any) {
	if native, ok := mapField(dataset, "native"); ok {
		return firstString(native, "query"), mapFieldOrEmpty(native, "template_tags", "template-tags")
	}

	for _, stage := range mapList(dataset, "stages") {
		var query string
		switch raw := stage["native"].(type) {
		case string:
			query = raw
		case map[string]any:
			query = firstString(raw, "query")
		}
		if query == "" {
			continue
		}
		tags := mapFieldOrEmpty(stage, "template_tags", "template-tags")
		if len(tags) == 0 {
			tags = mapFieldOrEmpty(stage, "template-tags")
		}
		return query, tags
	}
	return "", nil
}

func collectModelSQL(root, dashboardRoot map[string]any) map[string]string {
	models := map[string]string{}
	addCard := func(card map[string]any) {
		id := firstString(card, "id")
		if id == "" {
			return
		}
		if firstString(card, "type") != "model" && firstString(card, "query_type", "query-type") != "native" {
			return
		}
		sql, _ := nativeSQL(card, nil)
		if sql == "" {
			return
		}
		models[id] = sql
	}

	for _, source := range []map[string]any{root, dashboardRoot} {
		for _, key := range []string{"x-dac-metabase-source-cards", "source_cards", "source-cards", "models"} {
			if sourceCards, ok := mapField(source, key); ok {
				for _, raw := range sourceCards {
					if card, ok := raw.(map[string]any); ok {
						addCard(card)
					}
				}
			}
			for _, card := range mapList(source, key) {
				addCard(card)
			}
		}
	}
	for _, dashcard := range extractCards(dashboardRoot) {
		if card, ok := mapField(dashcard, "card", "question"); ok {
			addCard(card)
		}
	}
	return models
}

func collectMetabaseTables(root, dashboardRoot map[string]any) map[string]*metabaseTableInfo {
	tables := map[string]*metabaseTableInfo{}
	for _, source := range []map[string]any{root, dashboardRoot} {
		for _, key := range []string{"x-dac-metabase-tables", "metabase_tables", "metabase-tables"} {
			if rawTables, ok := mapField(source, key); ok {
				ids := make([]string, 0, len(rawTables))
				for id := range rawTables {
					ids = append(ids, id)
				}
				sort.Strings(ids)
				for _, id := range ids {
					if table, ok := rawTables[id].(map[string]any); ok {
						if info := parseMetabaseTable(table); info != nil {
							tables[info.ID] = info
						}
					}
				}
			}
			for _, table := range mapList(source, key) {
				if info := parseMetabaseTable(table); info != nil {
					tables[info.ID] = info
				}
			}
		}
	}
	return tables
}

func parseMetabaseTable(table map[string]any) *metabaseTableInfo {
	id := firstString(table, "id")
	name := firstString(table, "name")
	if id == "" || name == "" {
		return nil
	}
	info := &metabaseTableInfo{
		ID:         id,
		Name:       name,
		Schema:     firstString(table, "schema"),
		FieldNames: map[string]string{},
	}
	if info.Schema != "" {
		info.Expression = quoteIdent(info.Schema) + "." + quoteIdent(info.Name)
	} else {
		info.Expression = quoteIdent(info.Name)
	}
	for _, field := range mapList(table, "fields") {
		fieldName := firstString(field, "name")
		if fieldName == "" {
			continue
		}
		for _, alias := range []string{
			firstString(field, "id"),
			fieldName,
			firstString(field, "display_name", "display-name"),
			normalizeName(firstString(field, "display_name", "display-name")),
		} {
			if alias != "" {
				info.FieldNames[alias] = fieldName
			}
		}
		info.Columns = append(info.Columns, columnInfo{
			Name: fieldName,
			Type: strings.ToLower(firstString(field, "base_type", "base-type", "effective_type", "effective-type", "semantic_type", "semantic-type")),
		})
	}
	return info
}

func modelBackedSQL(card, cardRoot map[string]any, ctx conversionContext) (string, []string) {
	dataset, ok := mapField(card, "dataset_query", "dataset-query")
	if !ok {
		return "", nil
	}
	stages := mapList(dataset, "stages")
	if len(stages) == 0 {
		if query, ok := mapField(dataset, "query"); ok {
			stages = []map[string]any{query}
		}
	}
	if len(stages) == 0 {
		return "", nil
	}

	stage := stages[0]
	sourceID := sourceCardID(stage)
	if sourceID == "" {
		sourceID = sourceCardID(dataset)
	}
	baseSQL := ctx.modelSQL[sourceID]
	if baseSQL == "" {
		return "", nil
	}

	breakouts, ok := fieldList(stage["breakout"])
	if !ok {
		return "", []string{"unsupported MBQL breakout"}
	}
	aggregations, ok := aggregationList(stage["aggregation"], resultColumns(card), len(breakouts))
	if !ok {
		return "", []string{"unsupported MBQL aggregation"}
	}
	filterClauses, ok := mbqlFilterClauses(stage["filter"])
	if !ok {
		return "", []string{"unsupported MBQL filter"}
	}
	clauses := append(filterClauses, dashboardFilterClauses(cardRoot, ctx)...)
	if len(breakouts) == 0 && len(aggregations) == 0 {
		orderBy, ok := mbqlOrderByClauses(stage, nil, nil, fieldRefName)
		if !ok {
			return "", []string{"unsupported MBQL order-by"}
		}
		return selectFromModel(baseSQL, []string{"*"}, clauses, nil, orderBy, intField(stage, 0, "limit")), nil
	}

	selects := make([]string, 0, len(breakouts)+len(aggregations))
	groupBy := make([]string, 0, len(breakouts))
	for i, field := range breakouts {
		selects = append(selects, fmt.Sprintf("%s AS %s", quoteIdent(field), quoteIdent(field)))
		groupBy = append(groupBy, strconv.Itoa(i+1))
	}
	for _, agg := range aggregations {
		selects = append(selects, fmt.Sprintf("%s AS %s", agg.Expression, quoteIdent(agg.Alias)))
	}
	if len(selects) == 0 {
		selects = append(selects, "*")
	}

	display := strings.ToLower(firstString(card, "display", "display_type", "display-type", "type"))
	orderBy, ok := mbqlOrderByClauses(stage, breakouts, aggregations, fieldRefName)
	if !ok {
		return "", []string{"unsupported MBQL order-by"}
	}
	if len(orderBy) == 0 && len(breakouts) > 0 && chronologicalDisplay(display) {
		orderBy = append(orderBy, quoteIdent(breakouts[0])+" ASC")
	} else if len(orderBy) == 0 && len(aggregations) > 0 {
		orderBy = append(orderBy, quoteIdent(aggregations[0].Alias)+" DESC")
	}
	return selectFromModel(baseSQL, selects, clauses, groupBy, orderBy, intField(stage, 0, "limit")), nil
}

func physicalTableBackedSQL(card, cardRoot map[string]any, ctx conversionContext) (string, []string, string) {
	dataset, ok := mapField(card, "dataset_query", "dataset-query")
	if !ok {
		return "", nil, ""
	}
	stages := mapList(dataset, "stages")
	if len(stages) == 0 {
		if query, ok := mapField(dataset, "query"); ok {
			stages = []map[string]any{query}
		}
	}
	if len(stages) == 0 {
		return "", nil, ""
	}

	stage := stages[0]
	tableID := sourceTableID(stage)
	if tableID == "" {
		tableID = sourceTableID(dataset)
	}
	if tableID == "" {
		return "", nil, ""
	}
	tableInfo := ctx.tables[tableID]
	if tableInfo == nil {
		return "", nil, ""
	}

	columns := resultColumns(card)
	if len(columns) == 0 {
		columns = tableInfo.Columns
	}
	resolver := tableInfo.resolveFieldRef
	breakouts, ok := fieldListWithResolver(stage["breakout"], resolver)
	if !ok {
		return "", []string{"unsupported MBQL breakout"}, ""
	}
	aggregations, ok := aggregationListWithResolver(stage["aggregation"], columns, len(breakouts), resolver)
	if !ok {
		return "", []string{"unsupported MBQL aggregation"}, ""
	}
	filterClauses, ok := mbqlFilterClausesWithResolver(stage["filter"], resolver)
	if !ok {
		return "", []string{"unsupported MBQL filter"}, ""
	}
	clauses := append(filterClauses, dashboardFilterClausesWithResolver(cardRoot, ctx, resolver)...)

	limit := intField(stage, 0, "limit")
	if len(breakouts) == 0 && len(aggregations) == 0 {
		if limit == 0 {
			limit = 2000
		}
		orderBy, ok := mbqlOrderByClauses(stage, nil, nil, resolver)
		if !ok {
			return "", []string{"unsupported MBQL order-by"}, ""
		}
		return selectFromTable(tableInfo.Expression, []string{"*"}, clauses, nil, orderBy, limit), nil, "table"
	}

	selects := make([]string, 0, len(breakouts)+len(aggregations))
	groupBy := make([]string, 0, len(breakouts))
	for i, field := range breakouts {
		selects = append(selects, fmt.Sprintf("%s AS %s", quoteIdent(field), quoteIdent(field)))
		groupBy = append(groupBy, strconv.Itoa(i+1))
	}
	for _, agg := range aggregations {
		selects = append(selects, fmt.Sprintf("%s AS %s", agg.Expression, quoteIdent(agg.Alias)))
	}
	if len(selects) == 0 {
		return "", nil, ""
	}

	display := strings.ToLower(firstString(card, "display", "display_type", "display-type", "type"))
	orderBy, ok := mbqlOrderByClauses(stage, breakouts, aggregations, resolver)
	if !ok {
		return "", []string{"unsupported MBQL order-by"}, ""
	}
	if len(orderBy) == 0 {
		if len(breakouts) > 0 && chronologicalDisplay(display) {
			orderBy = append(orderBy, quoteIdent(breakouts[0])+" ASC")
		} else if len(aggregations) > 0 {
			orderBy = append(orderBy, quoteIdent(aggregations[0].Alias)+" DESC")
		}
	}
	return selectFromTable(tableInfo.Expression, selects, clauses, groupBy, orderBy, limit), nil, ""
}

func chronologicalDisplay(display string) bool {
	switch display {
	case "line", "area", "combo":
		return true
	default:
		return false
	}
}

type aggregation struct {
	Expression string
	Alias      string
}

func aggregationList(raw any, columns []columnInfo, breakoutCount int) ([]aggregation, bool) {
	return aggregationListWithResolver(raw, columns, breakoutCount, fieldRefName)
}

func aggregationListWithResolver(raw any, columns []columnInfo, breakoutCount int, resolveField func(any) string) ([]aggregation, bool) {
	if raw == nil {
		return nil, true
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	out := make([]aggregation, 0, len(list))
	for i, item := range list {
		parts, ok := item.([]any)
		if !ok || len(parts) == 0 {
			return nil, false
		}
		fn, _ := parts[0].(string)
		field := ""
		for _, part := range parts[1:] {
			if name := resolveField(part); name != "" {
				field = name
				break
			}
		}
		expr := ""
		switch fn {
		case "count":
			if field == "" {
				expr = "COUNT(*)"
			} else {
				expr = "COUNT(" + quoteIdent(field) + ")"
			}
		case "sum", "avg", "min", "max":
			if field == "" {
				return nil, false
			}
			expr = strings.ToUpper(fn) + "(" + quoteIdent(field) + ")"
		case "distinct", "count-distinct":
			if field == "" {
				return nil, false
			}
			expr = "COUNT(DISTINCT " + quoteIdent(field) + ")"
		default:
			return nil, false
		}
		alias := ""
		if breakoutCount+i < len(columns) {
			alias = columns[breakoutCount+i].Name
		}
		if alias == "" {
			alias = fn
			if field != "" && fn != "count" {
				alias += "_" + field
			}
		}
		out = append(out, aggregation{Expression: expr, Alias: alias})
	}
	return out, true
}

func selectFromTable(tableExpression string, selects, whereClauses, groupBy, orderBy []string, limit int) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(selects, ", "))
	b.WriteString("\nFROM ")
	b.WriteString(tableExpression)
	b.WriteString("\nWHERE 1=1")
	for _, clause := range whereClauses {
		if strings.TrimSpace(clause) == "" {
			continue
		}
		b.WriteString("\n  AND ")
		b.WriteString(clause)
	}
	if len(groupBy) > 0 {
		b.WriteString("\nGROUP BY ")
		b.WriteString(strings.Join(groupBy, ", "))
	}
	if len(orderBy) > 0 {
		b.WriteString("\nORDER BY ")
		b.WriteString(strings.Join(orderBy, ", "))
	}
	if limit > 0 {
		b.WriteString("\nLIMIT ")
		b.WriteString(strconv.Itoa(limit))
	}
	return b.String()
}

func selectFromModel(baseSQL string, selects, whereClauses, groupBy, orderBy []string, limit int) string {
	var b strings.Builder
	b.WriteString("WITH source AS (\n")
	b.WriteString(trimSQLTerminator(baseSQL))
	b.WriteString("\n)\nSELECT ")
	b.WriteString(strings.Join(selects, ", "))
	b.WriteString("\nFROM source\nWHERE 1=1")
	for _, clause := range whereClauses {
		if strings.TrimSpace(clause) == "" {
			continue
		}
		b.WriteString("\n  AND ")
		b.WriteString(clause)
	}
	if len(groupBy) > 0 {
		b.WriteString("\nGROUP BY ")
		b.WriteString(strings.Join(groupBy, ", "))
	}
	if len(orderBy) > 0 {
		b.WriteString("\nORDER BY ")
		b.WriteString(strings.Join(orderBy, ", "))
	}
	if limit > 0 {
		b.WriteString("\nLIMIT ")
		b.WriteString(strconv.Itoa(limit))
	}
	return b.String()
}

func trimSQLTerminator(sql string) string {
	sql = strings.TrimSpace(sql)
	for strings.HasSuffix(sql, ";") {
		sql = strings.TrimSpace(strings.TrimSuffix(sql, ";"))
	}
	return sql
}

func sourceTableID(m map[string]any) string {
	for _, key := range []string{"source-table", "source_table"} {
		switch v := m[key].(type) {
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case int:
			return strconv.Itoa(v)
		case json.Number:
			return v.String()
		case string:
			if strings.HasPrefix(v, "card__") {
				continue
			}
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func sourceCardID(m map[string]any) string {
	for _, key := range []string{"source-card", "source_card"} {
		switch v := m[key].(type) {
		case float64:
			return strconv.FormatInt(int64(v), 10)
		case int:
			return strconv.Itoa(v)
		case string:
			return strings.TrimPrefix(v, "card__")
		}
	}
	if sourceTable := firstString(m, "source-table", "source_table"); strings.HasPrefix(sourceTable, "card__") {
		return strings.TrimPrefix(sourceTable, "card__")
	}
	return ""
}

func dashboardFilterClauses(cardRoot map[string]any, ctx conversionContext) []string {
	return dashboardFilterClausesWithResolver(cardRoot, ctx, fieldRefName)
}

func dashboardFilterClausesWithResolver(cardRoot map[string]any, ctx conversionContext, resolveField func(any) string) []string {
	var clauses []string
	for _, mapping := range mapList(cardRoot, "parameter_mappings", "parameter-mappings") {
		if !parameterMappingAppliesToCard(mapping, cardRoot) {
			continue
		}
		filter, ok := filterForParameterMapping(mapping, ctx)
		if !ok {
			continue
		}
		field := targetFieldNameWithResolver(mapping["target"], resolveField)
		if field == "" {
			continue
		}
		if clause := filterClause(field, filter); clause != "" {
			clauses = append(clauses, clause)
		}
	}
	return clauses
}

func filterForParameterMapping(mapping map[string]any, ctx conversionContext) (dashboard.Filter, bool) {
	paramID := firstString(mapping, "parameter_id", "parameter-id")
	filterName := mappedFilterName(ctx.filterNames, paramID)
	if filterName == "" {
		filterName = normalizeName(paramID)
	}
	filter, ok := ctx.filters[filterName]
	return filter, ok
}

func parameterMappingAppliesToCard(mapping, cardRoot map[string]any) bool {
	if id := metabaseID(firstValue(mapping, "card_id", "card-id")); id != "" {
		return id == currentCardID(cardRoot)
	}
	if id := metabaseID(firstValue(mapping, "dashcard_id", "dashcard-id", "dashboard_card_id", "dashboard-card-id")); id != "" {
		return id == currentDashcardID(cardRoot)
	}
	return false
}

func currentCardID(cardRoot map[string]any) string {
	if id := metabaseID(firstValue(cardRoot, "card_id", "card-id")); id != "" {
		return id
	}
	for _, key := range []string{"card", "question", "virtual_card", "virtual-card"} {
		if child, ok := mapField(cardRoot, key); ok {
			if id := metabaseID(firstValue(child, "id")); id != "" {
				return id
			}
		}
	}
	if settings, ok := mapField(cardRoot, "visualization_settings", "visualization-settings"); ok {
		if virtual, ok := mapField(settings, "virtual_card", "virtual-card"); ok {
			if id := metabaseID(firstValue(virtual, "id")); id != "" {
				return id
			}
		}
	}
	return ""
}

func currentDashcardID(cardRoot map[string]any) string {
	return metabaseID(firstValue(cardRoot, "id"))
}

func metabaseID(v any) string {
	return strings.TrimPrefix(metabaseScalarID(v), "card__")
}

func mbqlFilterClauses(raw any) ([]string, bool) {
	return mbqlFilterClausesWithResolver(raw, fieldRefName)
}

func mbqlFilterClausesWithResolver(raw any, resolveField func(any) string) ([]string, bool) {
	if raw == nil {
		return nil, true
	}
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return nil, false
	}
	op, _ := parts[0].(string)
	if op == "and" {
		var clauses []string
		for _, child := range parts[1:] {
			childClauses, ok := mbqlFilterClausesWithResolver(child, resolveField)
			if !ok {
				return nil, false
			}
			clauses = append(clauses, childClauses...)
		}
		return clauses, true
	}
	if op == "is-null" || op == "not-null" {
		if len(parts) < 2 {
			return nil, false
		}
		field := resolveField(parts[1])
		if field == "" {
			return nil, false
		}
		if op == "is-null" {
			return []string{quoteIdent(field) + " IS NULL"}, true
		}
		return []string{quoteIdent(field) + " IS NOT NULL"}, true
	}
	if len(parts) < 3 {
		return nil, false
	}
	field := resolveField(parts[1])
	if field == "" {
		return nil, false
	}
	switch op {
	case "=":
		return []string{quoteIdent(field) + " = " + sqlLiteral(parts[2])}, true
	case "!=":
		return []string{quoteIdent(field) + " <> " + sqlLiteral(parts[2])}, true
	case "<", ">", "<=", ">=":
		return []string{quoteIdent(field) + " " + op + " " + sqlLiteral(parts[2])}, true
	case "between":
		if len(parts) < 4 {
			return nil, false
		}
		return []string{quoteIdent(field) + " BETWEEN " + sqlLiteral(parts[2]) + " AND " + sqlLiteral(parts[3])}, true
	default:
		return nil, false
	}
}

func filterClause(field string, filter dashboard.Filter) string {
	name := filter.Name
	ident := quoteIdent(field)
	switch filter.Type {
	case "date-range":
		return fmt.Sprintf("%s >= '{{ filters.%s.start }}' AND %s <= '{{ filters.%s.end }}'", ident, name, ident, name)
	case "date":
		return fmt.Sprintf("%s = '{{ filters.%s }}'", ident, name)
	case "number":
		return fmt.Sprintf("%s = {{ filters.%s }}", ident, name)
	case "select", "text":
		if filter.Multiple {
			return fmt.Sprintf("%s IN ('{{ filters.%s | join(\"','\") }}')", ident, name)
		}
		return fmt.Sprintf("%s = '{{ filters.%s }}'", ident, name)
	default:
		return ""
	}
}

func fieldList(raw any) ([]string, bool) {
	return fieldListWithResolver(raw, fieldRefName)
}

func fieldListWithResolver(raw any, resolveField func(any) string) ([]string, bool) {
	if raw == nil {
		return nil, true
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	var fields []string
	for _, item := range list {
		if metabaseTemporalGranularity(item) != "" {
			return nil, false
		}
		name := resolveField(item)
		if name == "" {
			return nil, false
		}
		fields = append(fields, name)
	}
	return fields, true
}

func targetFieldName(raw any) string {
	return targetFieldNameWithResolver(raw, fieldRefName)
}

func targetFieldNameWithResolver(raw any, resolveField func(any) string) string {
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	switch kind {
	case "dimension":
		for _, part := range parts[1:] {
			if name := resolveField(part); name != "" {
				return name
			}
		}
	}
	return ""
}

func targetTemplateTagName(raw any) string {
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	if kind == "template-tag" {
		if len(parts) < 2 {
			return ""
		}
		return metabaseID(parts[1])
	}
	for _, part := range parts[1:] {
		if name := targetTemplateTagName(part); name != "" {
			return name
		}
	}
	return ""
}

func fieldRefName(raw any) string {
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	if kind != "field" {
		return ""
	}
	for i := len(parts) - 1; i >= 1; i-- {
		if s, ok := parts[i].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (t *metabaseTableInfo) resolveFieldRef(raw any) string {
	if name := fieldRefName(raw); name != "" {
		if mapped := t.FieldNames[name]; mapped != "" {
			return mapped
		}
		return name
	}
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	if kind != "field" {
		return ""
	}
	for _, part := range parts[1:] {
		if id := metabaseScalarID(part); id != "" {
			if mapped := t.FieldNames[id]; mapped != "" {
				return mapped
			}
		}
	}
	return ""
}

func metabaseScalarID(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func mbqlOrderByClauses(stage map[string]any, breakouts []string, aggregations []aggregation, resolveField func(any) string) ([]string, bool) {
	raw, present := firstPresentValue(stage, "order-by", "order_by")
	if !present || raw == nil {
		return nil, true
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	var out []string
	for _, item := range list {
		parts, ok := item.([]any)
		if !ok || len(parts) < 2 {
			return nil, false
		}
		direction, _ := parts[0].(string)
		direction = strings.ToUpper(direction)
		if direction != "ASC" && direction != "DESC" {
			return nil, false
		}
		name := mbqlOrderByName(parts[1], breakouts, aggregations, resolveField)
		if name == "" {
			return nil, false
		}
		out = append(out, quoteIdent(name)+" "+direction)
	}
	return out, true
}

func mbqlOrderByName(raw any, breakouts []string, aggregations []aggregation, resolveField func(any) string) string {
	if name := resolveField(raw); name != "" {
		return name
	}
	parts, ok := raw.([]any)
	if !ok || len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	switch kind {
	case "aggregation":
		if len(parts) < 2 {
			return ""
		}
		idx, ok := intValue(parts[1])
		if !ok || idx < 0 || idx >= len(aggregations) {
			return ""
		}
		return aggregations[idx].Alias
	case "breakout":
		if len(parts) < 2 {
			return ""
		}
		idx, ok := intValue(parts[1])
		if !ok || idx < 0 || idx >= len(breakouts) {
			return ""
		}
		return breakouts[idx]
	default:
		return ""
	}
}

func intValue(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		i, err := x.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func quoteIdent(name string) string {
	if regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(name) {
		return `"` + name + `"`
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqlLiteral(v any) string {
	switch x := v.(type) {
	case string:
		return "'" + strings.ReplaceAll(x, "'", "''") + "'"
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprint(x), "'", "''") + "'"
	}
}

var metabaseVarPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)
var metabaseOptionalPattern = regexp.MustCompile(`(?s)\[\[(.*?)\]\]`)

func convertSQLTemplate(sql string, variables map[string]sqlTemplateVariable) (string, int, []string) {
	optionalCount := strings.Count(sql, "[[")
	sql = convertOptionalSQLClauses(sql, variables)
	matches := metabaseVarPattern.FindAllStringSubmatchIndex(sql, -1)
	if len(matches) == 0 {
		return sql, optionalCount, nil
	}

	var b strings.Builder
	last := 0
	unresolvedSeen := map[string]bool{}
	var unresolved []string
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		start, end := match[0], match[1]
		name := sql[match[2]:match[3]]
		b.WriteString(sql[last:start])
		variable, ok := sqlTemplateVariableForName(name, variables)
		if !ok {
			if !unresolvedSeen[name] {
				unresolvedSeen[name] = true
				unresolved = append(unresolved, name)
			}
			b.WriteString(nullTemplateReplacement(sql, start, end))
			last = end
			continue
		}
		replacement := ""
		if variable.UseFilter {
			replacement = "{{ filters." + variable.FilterName + " }}"
			if variable.Quote && !insideSingleQuotedString(sql, start) {
				replacement = "'" + replacement + "'"
			}
		} else if variable.HasDefault {
			replacement = sqlTemplateDefaultLiteral(variable.Default, variable.Quote, insideSingleQuotedString(sql, start))
		} else {
			if !unresolvedSeen[name] {
				unresolvedSeen[name] = true
				unresolved = append(unresolved, name)
			}
			replacement = nullTemplateReplacement(sql, start, end)
		}
		b.WriteString(replacement)
		last = end
	}
	b.WriteString(sql[last:])
	return b.String(), optionalCount, unresolved
}

func convertOptionalSQLClauses(sql string, variables map[string]sqlTemplateVariable) string {
	matches := metabaseOptionalPattern.FindAllStringSubmatchIndex(sql, -1)
	if len(matches) == 0 {
		return sql
	}
	var b strings.Builder
	last := 0
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		b.WriteString(sql[last:match[0]])
		body := sql[match[2]:match[3]]
		if optionalClauseHasValues(body, variables) {
			b.WriteString(body)
		}
		last = match[1]
	}
	b.WriteString(sql[last:])
	return b.String()
}

func optionalClauseHasValues(sql string, variables map[string]sqlTemplateVariable) bool {
	matches := metabaseVarPattern.FindAllStringSubmatch(sql, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		variable, ok := sqlTemplateVariableForName(match[1], variables)
		if !ok || (!variable.UseFilter && !variable.HasDefault) {
			return false
		}
	}
	return true
}

func sqlTemplateVariableForName(name string, variables map[string]sqlTemplateVariable) (sqlTemplateVariable, bool) {
	if variables == nil {
		return sqlTemplateVariable{}, false
	}
	if variable, ok := variables[name]; ok {
		return variable, true
	}
	variable, ok := variables[normalizeName(name)]
	return variable, ok
}

func sqlTemplateDefaultLiteral(value any, quote bool, alreadyQuoted bool) string {
	if alreadyQuoted && quote {
		return strings.ReplaceAll(fmt.Sprint(value), "'", "''")
	}
	if quote {
		return sqlLiteral(value)
	}
	switch x := value.(type) {
	case string:
		return strings.ReplaceAll(x, "'", "''")
	default:
		return sqlLiteral(value)
	}
}

func nullTemplateReplacement(sql string, start, end int) string {
	if insideSingleQuotedString(sql, start) {
		return ""
	}
	return "NULL"
}

func insideSingleQuotedString(sql string, pos int) bool {
	inString := false
	for i := 0; i < pos && i < len(sql); i++ {
		if sql[i] != '\'' {
			continue
		}
		if inString && i+1 < pos && sql[i+1] == '\'' {
			i++
			continue
		}
		inString = !inString
	}
	return inString
}

func widgetForDisplay(name, display, sql string, settings map[string]any, columns []columnInfo) (dashboard.Widget, []string) {
	base := dashboard.Widget{
		Name: name,
		SQL:  sql,
		Col:  defaultWidthForDisplay(display),
	}

	switch display {
	case "scalar", "smartscalar":
		field := firstNumericColumn(columns)
		if field == "" {
			field = firstColumn(columns)
		}
		base.Type = dashboard.WidgetTypeMetric
		base.Value = &dashboard.ValueEncoding{Field: field, Type: valueType(field, columns), Format: numberFormat(field, columns)}
		return base, nil

	case "line", "area", "bar", "row", "combo", "scatter", "waterfall":
		base.Type = dashboard.WidgetTypeChart
		base.Chart = chartType(display)
		base.X, base.Y = xyEncodings(display, settings, columns)
		if display == "row" {
			base.Horizontal = true
		}
		if display == "combo" {
			base.Lines = comboLineFields(base.Y)
		}
		return base, nil

	case "pie", "funnel", "treemap":
		base.Type = dashboard.WidgetTypeChart
		base.Chart = display
		base.Label = firstSettingString(settings, []string{"pie.dimension", "funnel.dimension", "treemap.label", "graph.dimensions"})
		if base.Label == "" {
			base.Label = firstCategoryColumn(columns)
		}
		if base.Label == "" {
			base.Label = "label"
		}
		value := firstSettingString(settings, []string{"pie.metric", "funnel.metric", "treemap.value", "graph.metrics"})
		if value == "" {
			value = firstNumericColumn(columns)
		}
		if value == "" {
			value = "value"
		}
		base.Value = &dashboard.ValueEncoding{Field: value, Type: "number", Format: numberFormat(value, columns)}
		return base, nil

	case "gauge", "progress":
		base.Type = dashboard.WidgetTypeChart
		base.Chart = "gauge"
		field := firstNumericColumn(columns)
		if field == "" {
			field = firstColumn(columns)
		}
		base.Value = &dashboard.ValueEncoding{Field: field, Type: "number", Format: numberFormat(field, columns)}
		base.Target = firstSettingString(settings, []string{"gauge.target", "gauge.max"})
		return base, nil

	case "table", "object":
		base.Type = dashboard.WidgetTypeTable
		base.Col = 12
		base.Columns = tableColumns(columns)
		return base, nil

	case "pivot", "map":
		base.Type = dashboard.WidgetTypeTable
		base.Col = 12
		base.Columns = tableColumns(columns)
		return base, []string{fmt.Sprintf("display type %q was mapped to a table", display)}

	default:
		base.Type = dashboard.WidgetTypeTable
		base.Col = 12
		base.Columns = tableColumns(columns)
		if display == "" {
			return base, []string{"missing Metabase display type was mapped to a table"}
		}
		return base, []string{fmt.Sprintf("unsupported display type %q was mapped to a table", display)}
	}
}

func chartType(display string) string {
	switch display {
	case "row":
		return "bar"
	default:
		return display
	}
}

func xyEncodings(display string, settings map[string]any, columns []columnInfo) (*dashboard.AxisEncoding, *dashboard.AxisEncoding) {
	if display == "scatter" {
		numeric := numericColumns(columns)
		x := firstColumn(columns)
		y := ""
		if len(numeric) > 0 {
			x = numeric[0]
		}
		if len(numeric) > 1 {
			y = numeric[1]
		} else if len(columns) > 1 {
			y = columns[1].Name
		} else {
			y = x
		}
		return axisForColumn(x, columns, false), axisForColumnList([]string{y}, columns)
	}

	x := firstSettingString(settings, []string{"graph.dimensions", "graph.x_axis", "graph.x-axis"})
	if x == "" {
		x = firstDateColumn(columns)
	}
	if x == "" {
		x = firstCategoryColumn(columns)
	}
	if x == "" {
		x = firstColumn(columns)
	}

	y := firstSettingStringList(settings, []string{"graph.metrics", "graph.y_axis", "graph.y-axis"})
	if len(y) == 0 {
		for _, col := range numericColumns(columns) {
			if col != x {
				y = append(y, col)
			}
		}
	}
	if len(y) == 0 {
		for _, col := range columns {
			if col.Name != x {
				y = append(y, col.Name)
				break
			}
		}
	}
	if len(y) == 0 && x != "" {
		y = []string{x}
	}
	return axisForColumn(x, columns, false), axisForColumnList(y, columns)
}

func comboLineFields(y *dashboard.AxisEncoding) []string {
	fields := y.FieldList()
	if len(fields) < 2 {
		return nil
	}
	return []string{fields[len(fields)-1]}
}

func axisForColumn(name string, columns []columnInfo, list bool) *dashboard.AxisEncoding {
	if name == "" {
		return nil
	}
	field := any(name)
	if list {
		field = []string{name}
	}
	return &dashboard.AxisEncoding{Field: field, Type: valueType(name, columns), Format: axisFormat(name, columns)}
}

func axisForColumnList(names []string, columns []columnInfo) *dashboard.AxisEncoding {
	if len(names) == 0 {
		return nil
	}
	return &dashboard.AxisEncoding{Field: names, Type: "number", Format: numberFormat(names[0], columns)}
}

func resultColumns(card map[string]any) []columnInfo {
	for _, key := range []string{"result_metadata", "result-metadata", "metadata"} {
		if list := mapList(card, key); len(list) > 0 {
			return parseColumns(list)
		}
	}
	if dataset, ok := mapField(card, "dataset_query", "dataset-query"); ok {
		if list := mapList(dataset, "result_metadata", "result-metadata", "metadata"); len(list) > 0 {
			return parseColumns(list)
		}
	}
	return inferColumnsFromSQL(card)
}

var sqlAliasPattern = regexp.MustCompile(`(?i)\bas\s+"?([A-Za-z_][A-Za-z0-9_]*)"?`)

func inferColumnsFromSQL(card map[string]any) []columnInfo {
	sql, _ := nativeSQL(card, nil)
	if sql == "" {
		return nil
	}
	matches := sqlAliasPattern.FindAllStringSubmatch(sql, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := map[string]bool{}
	cols := make([]columnInfo, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" || seen[match[1]] {
			continue
		}
		cols = append(cols, columnInfo{Name: match[1]})
		seen[match[1]] = true
	}
	return cols
}

func parseColumns(list []map[string]any) []columnInfo {
	cols := make([]columnInfo, 0, len(list))
	for _, item := range list {
		name := firstString(item, "name", "field_ref", "field-ref", "display_name", "display-name")
		if strings.HasPrefix(name, "[") {
			name = ""
		}
		if name == "" {
			continue
		}
		cols = append(cols, columnInfo{
			Name: name,
			Type: strings.ToLower(firstString(item, "base_type", "base-type", "semantic_type", "semantic-type", "effective_type", "effective-type")),
		})
	}
	return cols
}

func tableColumns(columns []columnInfo) []dashboard.TableColumn {
	out := make([]dashboard.TableColumn, 0, len(columns))
	for _, col := range columns {
		out = append(out, dashboard.TableColumn{Name: col.Name, Label: titleFromName(col.Name)})
	}
	return out
}

func textContent(cardRoot, card, settings map[string]any) string {
	for _, source := range []map[string]any{cardRoot, card, settings} {
		if text := firstString(source, "text", "content", "body", "markdown"); text != "" {
			return text
		}
	}
	return ""
}

func cardName(cardRoot map[string]any, index int) string {
	card, _ := mapField(cardRoot, "card", "question")
	for _, source := range []map[string]any{cardRoot, card} {
		if source == nil {
			continue
		}
		if name := firstString(source, "name", "display_name", "display-name", "title"); name != "" {
			return name
		}
	}
	return fmt.Sprintf("Metabase Card %d", index)
}

func layoutRows(widgets []positionedWidget) []dashboard.Row {
	if len(widgets) == 0 {
		return []dashboard.Row{{
			Widgets: []dashboard.Widget{{
				Name:    "Empty Metabase Dashboard",
				Type:    dashboard.WidgetTypeText,
				Content: "No Metabase cards were found.",
				Col:     12,
			}},
		}}
	}

	gridWidth := inferGridWidth(widgets)
	for i := range widgets {
		widgets[i].w.Col = scaleWidth(widgets[i].sizeX, gridWidth)
	}

	sort.SliceStable(widgets, func(i, j int) bool {
		if widgets[i].row != widgets[j].row {
			return widgets[i].row < widgets[j].row
		}
		return widgets[i].col < widgets[j].col
	})

	var rows []dashboard.Row
	var current []dashboard.Widget
	currentRow := widgets[0].row
	currentCols := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		rows = append(rows, dashboard.Row{Widgets: current})
		current = nil
		currentCols = 0
	}

	for _, item := range widgets {
		if item.row != currentRow || currentCols+item.w.Col > 12 {
			flush()
			currentRow = item.row
		}
		current = append(current, item.w)
		currentCols += item.w.Col
	}
	flush()
	return rows
}

func inferGridWidth(widgets []positionedWidget) int {
	maxExtent := 12
	for _, w := range widgets {
		extent := w.col + w.sizeX
		if extent > maxExtent {
			maxExtent = extent
		}
	}
	if maxExtent <= 12 {
		return 12
	}
	if maxExtent <= 24 {
		return 24
	}
	return maxExtent
}

func scaleWidth(width, gridWidth int) int {
	if width <= 0 {
		return 12
	}
	scaled := int(math.Ceil(float64(width) * 12 / float64(gridWidth)))
	if scaled < 1 {
		return 1
	}
	if scaled > 12 {
		return 12
	}
	return scaled
}

func defaultWidthForWidget(w dashboard.Widget) int {
	if w.Type == dashboard.WidgetTypeMetric {
		return 6
	}
	if w.Type == dashboard.WidgetTypeTable || w.Type == dashboard.WidgetTypeText {
		return 12
	}
	return 8
}

func defaultWidthForDisplay(display string) int {
	switch display {
	case "scalar", "smartscalar", "gauge", "progress":
		return 3
	case "table", "pivot", "object":
		return 12
	default:
		return 6
	}
}

func firstColumn(columns []columnInfo) string {
	if len(columns) == 0 {
		return "value"
	}
	return columns[0].Name
}

func firstNumericColumn(columns []columnInfo) string {
	for _, col := range columns {
		if columnValueType(col) == "number" {
			return col.Name
		}
	}
	return ""
}

func firstDateColumn(columns []columnInfo) string {
	for _, col := range columns {
		if columnValueType(col) == "date" {
			return col.Name
		}
	}
	return ""
}

func firstCategoryColumn(columns []columnInfo) string {
	for _, col := range columns {
		if columnValueType(col) == "category" {
			return col.Name
		}
	}
	return ""
}

func numericColumns(columns []columnInfo) []string {
	var out []string
	for _, col := range columns {
		if columnValueType(col) == "number" {
			out = append(out, col.Name)
		}
	}
	return out
}

func isNumericType(t string) bool {
	return strings.Contains(t, "number") ||
		strings.Contains(t, "integer") ||
		strings.Contains(t, "float") ||
		strings.Contains(t, "decimal")
}

func isDateType(t string) bool {
	return strings.Contains(t, "date") || strings.Contains(t, "time")
}

func valueType(name string, columns []columnInfo) string {
	for _, col := range columns {
		if col.Name != name {
			continue
		}
		return columnValueType(col)
	}
	return ""
}

func columnValueType(col columnInfo) string {
	if isDateType(col.Type) {
		return "date"
	}
	if isNumericType(col.Type) {
		return "number"
	}
	if inferred := inferValueTypeFromName(col.Name); inferred != "" {
		return inferred
	}
	return "category"
}

func inferValueTypeFromName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "date"), strings.Contains(lower, "time"), strings.HasSuffix(lower, "_at"):
		return "date"
	case strings.Contains(lower, "count"), strings.Contains(lower, "num"),
		strings.Contains(lower, "total"), strings.Contains(lower, "sum"),
		strings.Contains(lower, "avg"), strings.Contains(lower, "amount"),
		strings.Contains(lower, "revenue"), strings.Contains(lower, "profit"),
		strings.Contains(lower, "cost"), strings.Contains(lower, "orders"),
		strings.Contains(lower, "sales"), strings.Contains(lower, "price"),
		strings.Contains(lower, "rate"), strings.Contains(lower, "pct"),
		strings.Contains(lower, "percent"):
		return "number"
	default:
		return ""
	}
}

func axisFormat(name string, columns []columnInfo) string {
	if valueType(name, columns) == "date" {
		return "%Y-%m-%d"
	}
	return numberFormat(name, columns)
}

func numberFormat(name string, columns []columnInfo) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "pct"), strings.Contains(lower, "percent"), strings.Contains(lower, "rate"):
		return ".1%"
	case strings.Contains(lower, "count"), strings.HasPrefix(lower, "num_"), strings.HasSuffix(lower, "_num"):
		return ",.0f"
	default:
		return ""
	}
}

func dacFilterType(sourceType string) string {
	t := strings.ToLower(sourceType)
	switch {
	case strings.Contains(t, "date") && (strings.Contains(t, "range") || strings.Contains(t, "relative") || strings.Contains(t, "all-options")):
		return "date-range"
	case strings.Contains(t, "date"):
		return "date"
	case strings.Contains(t, "number"), strings.Contains(t, "integer"), strings.Contains(t, "float"), strings.Contains(t, "decimal"):
		return "number"
	case strings.Contains(t, "category"), strings.Contains(t, "select"):
		return "select"
	default:
		return "text"
	}
}

func filterTypeNeedsQuotes(filterType string) bool {
	switch filterType {
	case "date", "date-range", "select", "text":
		return true
	default:
		return false
	}
}

func normalizeFilterDefault(v any, filterType string) any {
	if v == nil {
		return nil
	}
	t := strings.ToLower(filterType)
	if strings.Contains(t, "date") {
		if s, ok := v.(string); ok {
			return metabaseDateDefault(s, dacFilterType(t))
		}
		return nil
	}
	return v
}

func metabaseDateDefault(value, filterType string) any {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return nil
	}
	if filterType == "date" {
		switch v {
		case "today":
			return "TODAY"
		case "yesterday":
			return "TODAY-1"
		}
		if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(value) {
			return value
		}
		return nil
	}
	switch v {
	case "past7days", "past7days~", "last7days", "last_7_days":
		return "last_7_days"
	case "past30days", "past30days~", "last30days", "last_30_days":
		return "last_30_days"
	case "past90days", "past90days~", "last90days", "last_90_days":
		return "last_90_days"
	case "today":
		return "today"
	case "yesterday":
		return "yesterday"
	}
	if filterType == "date-range" {
		if start, end, ok := metabaseFixedDateRange(value); ok {
			return map[string]any{"start": start, "end": end}
		}
	}
	if regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(value) {
		return value
	}
	return nil
}

func metabaseFixedDateRange(value string) (string, string, bool) {
	value = strings.TrimSpace(value)
	datePattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	if datePattern.MatchString(value) {
		return value, value, true
	}
	start, end, found := strings.Cut(value, "~")
	if !found {
		return "", "", false
	}
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	if !datePattern.MatchString(start) || !datePattern.MatchString(end) {
		return "", "", false
	}
	return start, end, true
}

func extractStaticFilterValues(m map[string]any) []string {
	for _, key := range []string{"values", "options"} {
		if raw, ok := m[key]; ok {
			if values := stringList(raw); len(values) > 0 {
				return values
			}
		}
	}
	if config, ok := mapField(m, "values_source_config", "values-source-config"); ok {
		for _, key := range []string{"values", "options"} {
			if raw, ok := config[key]; ok {
				if values := stringList(raw); len(values) > 0 {
					return values
				}
			}
		}
	}
	return nil
}

func mapField(m map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			child, ok := v.(map[string]any)
			if ok {
				return child, true
			}
		}
	}
	return nil, false
}

func mapFieldOrEmpty(m map[string]any, keys ...string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	if child, ok := mapField(m, keys...); ok {
		return child
	}
	return map[string]any{}
}

func mapList(m map[string]any, keys ...string) []map[string]any {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok {
			continue
		}
		list, ok := raw.([]any)
		if !ok {
			continue
		}
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			if child, ok := item.(map[string]any); ok {
				out = append(out, child)
			}
		}
		return out
	}
	return nil
}

func firstValue(m map[string]any, keys ...string) any {
	value, _ := firstPresentValue(m, keys...)
	return value
}

func firstPresentValue(m map[string]any, keys ...string) (any, bool) {
	if m == nil {
		return nil, false
	}
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v, true
		}
	}
	return nil, false
}

func firstString(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, key := range keys {
		v, ok := m[key]
		if !ok || v == nil {
			continue
		}
		switch x := v.(type) {
		case string:
			return x
		case json.Number:
			return x.String()
		case float64:
			if x == math.Trunc(x) {
				return strconv.FormatInt(int64(x), 10)
			}
			return strconv.FormatFloat(x, 'f', -1, 64)
		case int:
			return strconv.Itoa(x)
		}
	}
	return ""
}

func intField(m map[string]any, fallback int, keys ...string) int {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		switch x := v.(type) {
		case int:
			return x
		case int64:
			return int(x)
		case float64:
			return int(x)
		case json.Number:
			i, err := x.Int64()
			if err == nil {
				return int(i)
			}
		case string:
			i, err := strconv.Atoi(x)
			if err == nil {
				return i
			}
		}
	}
	return fallback
}

func mergeMaps(left, right map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range left {
		out[k] = v
	}
	for k, v := range right {
		out[k] = v
	}
	return out
}

func firstSettingString(settings map[string]any, paths []string) string {
	for _, path := range paths {
		if values := firstSettingStringList(settings, []string{path}); len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func firstSettingStringList(settings map[string]any, paths []string) []string {
	for _, path := range paths {
		if raw, ok := settings[path]; ok {
			if values := stringList(raw); len(values) > 0 {
				return values
			}
			if s, ok := raw.(string); ok && s != "" {
				return []string{s}
			}
		}
		if raw, ok := nestedValue(settings, strings.Split(path, ".")); ok {
			if values := stringList(raw); len(values) > 0 {
				return values
			}
			if s, ok := raw.(string); ok && s != "" {
				return []string{s}
			}
		}
	}
	return nil
}

func nestedValue(m map[string]any, path []string) (any, bool) {
	var current any = m
	for _, part := range path {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok := asMap[part]
		if !ok {
			return nil, false
		}
		current = value
	}
	return current, true
}

func stringList(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			switch x := item.(type) {
			case string:
				if x != "" {
					out = append(out, x)
				}
			case map[string]any:
				if s := firstString(x, "name", "value", "display_name", "display-name"); s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

var nonNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func normalizeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = nonNameChars.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	s = strings.ToLower(s)
	if s == "" {
		return ""
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "filter_" + s
	}
	return s
}

func titleFromName(s string) string {
	parts := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " "))
	for i := range parts {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, " ")
}
