package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bruin-data/dac/schemas"
	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

// jsxTags are the built-in JSX tag names registered as string globals in goja.
// esbuild converts <Tag> to h(Tag, ...) where Tag is a variable reference;
// we define each as a string constant so h() receives a string.
var jsxTags = []string{
	"Dashboard", "Row", "Filter", "Query",
	"Metric", "Chart", "Table", "Text", "Divider", "Image",
	"Tabs", "Tab",
}

// LoadTSXFile loads a single .dashboard.tsx file by transpiling it with esbuild
// and executing it with goja to produce a Dashboard struct.
func LoadTSXFile(path string, opts ...TSXOption) (*Dashboard, error) {
	paths := ResolveProjectPathsForFile(path)
	semanticModels, err := loadSemanticModels(paths)
	if err != nil {
		return nil, err
	}
	return loadTSXFileWithContext(path, paths, semanticModels, opts...)
}

func loadTSXFileWithContext(path string, paths ProjectPaths, semanticModels semanticModelSet, opts ...TSXOption) (*Dashboard, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var cfg tsxConfig
	for _, o := range opts {
		o(&cfg)
	}

	d, err := evalTSX(string(source), path, &cfg)
	if err != nil {
		return nil, err
	}

	d.FilePath = path
	d.FileType = "tsx"
	d.SetProjectContext(paths.RootDir, semanticModels.models, semanticModels.invalid)

	// Run the same post-processing as YAML loader.
	postProcessDashboard(d)

	return d, nil
}

// TSXOption configures TSX loading behavior.
type TSXOption func(*tsxConfig)

type tsxConfig struct {
	queryFn func(connection, sql string) (map[string]interface{}, error)
}

// WithQueryFunc provides a query function for load-time SQL execution.
func WithQueryFunc(fn func(connection, sql string) (map[string]interface{}, error)) TSXOption {
	return func(c *tsxConfig) {
		c.queryFn = fn
	}
}

// transpileTSX uses esbuild to convert TSX source into JS with h() calls.
func transpileTSX(source string) (string, error) {
	result := api.Transform(source, api.TransformOptions{
		Loader:            api.LoaderTSX,
		JSXFactory:        "h",
		JSXFragment:       `""`,
		Format:            api.FormatCommonJS,
		Target:            api.ES2020,
		Platform:          api.PlatformNeutral,
		LegalComments:     api.LegalCommentsNone,
		MinifyWhitespace:  false,
		MinifyIdentifiers: false,
		MinifySyntax:      false,
	})
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Text
		}
		return "", fmt.Errorf("esbuild transpile: %s", strings.Join(msgs, "; "))
	}
	return string(result.Code), nil
}

// evalTSX transpiles and executes a TSX source, returning the Dashboard struct.
func evalTSX(source, filePath string, cfg *tsxConfig) (*Dashboard, error) {
	js, err := transpileTSX(source)
	if err != nil {
		return nil, err
	}

	vm := goja.New()

	// Build the h() / createElement function bound to this vm.
	hFunc := makeCreateElement(vm)

	// Register the createElement function (h).
	if err := vm.Set("h", hFunc); err != nil {
		return nil, fmt.Errorf("registering h: %w", err)
	}

	// Register JSX tag names as string globals.
	// esbuild treats capitalized JSX tags as variable references (React convention),
	// so <Dashboard> becomes h(Dashboard, ...) — we define Dashboard = "Dashboard"
	// so h() receives a string tag.
	for _, tag := range jsxTags {
		_ = vm.Set(tag, tag)
	}

	// Register console.log.
	console := vm.NewObject()
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			args[i] = a.String()
		}
		fmt.Println("[tsx]", strings.Join(args, " "))
		return goja.Undefined()
	})
	_ = vm.Set("console", console)

	// Register include() for reading .sql files.
	baseDir := filepath.Dir(filePath)
	_ = vm.Set("include", func(call goja.FunctionCall) goja.Value {
		relPath := call.Argument(0).String()
		content, err := readQueryFile(baseDir, relPath)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return vm.ToValue(content)
	})

	// Register require() for importing modules.
	moduleCache := make(map[string]goja.Value)
	_ = vm.Set("require", makeRequireFunc(vm, baseDir, moduleCache, cfg, hFunc))

	// Register query(). When a backend is available, executes SQL at load time.
	// Without a backend (e.g. `dac validate`), returns empty results so
	// the dashboard struct can still be validated.
	_ = vm.Set("query", func(call goja.FunctionCall) goja.Value {
		conn := call.Argument(0).String()
		sql := call.Argument(1).String()

		if cfg != nil && cfg.queryFn != nil {
			result, err := cfg.queryFn(conn, sql)
			if err != nil {
				panic(vm.ToValue(err.Error()))
			}
			return vm.ToValue(result)
		}

		// No backend — return empty result so the file still loads.
		return vm.ToValue(map[string]interface{}{
			"columns": []interface{}{},
			"rows":    []interface{}{},
		})
	})

	// Wrap in a module wrapper to capture exports.
	wrapped := "(function(exports, module) {\n" + js + "\n})"

	compiled, err := goja.Compile(filePath, wrapped, false)
	if err != nil {
		return nil, fmt.Errorf("compiling JS: %w", err)
	}

	fnVal, err := vm.RunProgram(compiled)
	if err != nil {
		return nil, fmt.Errorf("running JS: %w", err)
	}

	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		return nil, fmt.Errorf("expected wrapper function")
	}

	exports := vm.NewObject()
	module := vm.NewObject()
	_ = module.Set("exports", exports)

	_, err = fn(goja.Undefined(), exports, module)
	if err != nil {
		return nil, fmt.Errorf("executing module: %w", err)
	}

	// esbuild converts `export default X` to `exports.default = X`.
	// Check exports.default for the dashboard vnode.
	var nodeVal goja.Value

	if def := exports.Get("default"); def != nil && !goja.IsUndefined(def) && !goja.IsNull(def) {
		nodeVal = def
	} else {
		// Check if module.exports was reassigned (CommonJS style).
		modExports := module.Get("exports")
		if modExports != nil && !goja.IsUndefined(modExports) && !goja.IsNull(modExports) {
			// If module.exports is an object with a default key, use that.
			if obj := modExports.ToObject(vm); obj != nil {
				if def := obj.Get("default"); def != nil && !goja.IsUndefined(def) && !goja.IsNull(def) {
					nodeVal = def
				}
			}
		}
	}

	if nodeVal == nil {
		return nil, fmt.Errorf("no default export found")
	}

	node, err := extractVNode(nodeVal)
	if err != nil {
		return nil, fmt.Errorf("extracting dashboard: %w", err)
	}

	d, err := vnodeToDashboard(node)
	if err != nil {
		return nil, err
	}

	return d, nil
}

// vnode represents a virtual DOM node produced by h().
type vnode struct {
	Tag      string
	Props    map[string]interface{}
	Children []*vnode
}

// makeCreateElement builds an h(tag, props, ...children) function bound to the given runtime.
func makeCreateElement(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		tagArg := call.Argument(0)
		propsArg := call.Argument(1)

		// Collect children (arguments 2+).
		var children []interface{}
		for i := 2; i < len(call.Arguments); i++ {
			children = append(children, call.Arguments[i].Export())
		}

		// If tag is a function (custom component), call it with props.
		if fn, ok := goja.AssertFunction(tagArg); ok {
			// Build props object with children.
			props := vm.NewObject()
			if !goja.IsUndefined(propsArg) && !goja.IsNull(propsArg) {
				propsObj := propsArg.ToObject(vm)
				for _, key := range propsObj.Keys() {
					_ = props.Set(key, propsObj.Get(key))
				}
			}
			if len(children) > 0 {
				_ = props.Set("children", vm.ToValue(children))
			}

			result, err := fn(goja.Undefined(), props)
			if err != nil {
				panic(vm.ToValue(fmt.Sprintf("component error: %v", err)))
			}
			return result
		}

		// String tag — build a vnode.
		tag := tagArg.String()
		node := map[string]interface{}{
			"__vnode": true,
			"tag":     tag,
		}

		if !goja.IsUndefined(propsArg) && !goja.IsNull(propsArg) {
			node["props"] = propsArg.Export()
		} else {
			node["props"] = map[string]interface{}{}
		}

		// Flatten children (handles arrays from .map()).
		var flat []interface{}
		for _, c := range children {
			flattenChildren(c, &flat)
		}
		node["children"] = flat

		return vm.ToValue(node)
	}
}

// flattenChildren recursively flattens arrays of children.
func flattenChildren(v interface{}, out *[]interface{}) {
	if v == nil {
		return
	}
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			flattenChildren(item, out)
		}
	case map[string]interface{}:
		*out = append(*out, val)
	default:
		*out = append(*out, val)
	}
}

// extractVNode converts a goja Value into a vnode struct.
func extractVNode(val goja.Value) (*vnode, error) {
	exported := val.Export()
	m, ok := exported.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected vnode object, got %T", exported)
	}
	if _, ok := m["__vnode"]; !ok {
		return nil, fmt.Errorf("not a vnode (missing __vnode marker)")
	}

	node := &vnode{
		Tag:   asString(m["tag"]),
		Props: asMap(m["props"]),
	}

	if children, ok := m["children"]; ok {
		if arr, ok := children.([]interface{}); ok {
			for _, c := range arr {
				cm, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				if _, isVNode := cm["__vnode"]; !isVNode {
					continue
				}
				child, err := extractVNodeFromMap(cm)
				if err != nil {
					return nil, err
				}
				node.Children = append(node.Children, child)
			}
		}
	}

	return node, nil
}

func extractVNodeFromMap(m map[string]interface{}) (*vnode, error) {
	node := &vnode{
		Tag:   asString(m["tag"]),
		Props: asMap(m["props"]),
	}

	if children, ok := m["children"]; ok {
		if arr, ok := children.([]interface{}); ok {
			for _, c := range arr {
				cm, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				if _, isVNode := cm["__vnode"]; !isVNode {
					continue
				}
				child, err := extractVNodeFromMap(cm)
				if err != nil {
					return nil, err
				}
				node.Children = append(node.Children, child)
			}
		}
	}

	return node, nil
}

// vnodeToDashboard converts the root vnode tree into a Dashboard struct.
func vnodeToDashboard(root *vnode) (*Dashboard, error) {
	if root.Tag != "Dashboard" {
		return nil, fmt.Errorf("root element must be <Dashboard>, got <%s>", root.Tag)
	}

	d := &Dashboard{
		Schema:      schemas.DashboardV1ID,
		Name:        asString(root.Props["name"]),
		Description: asString(root.Props["description"]),
		Connection:  asString(root.Props["connection"]),
		Model:       asString(root.Props["model"]),
		Models:      asStringMap(root.Props["models"]),
	}

	for _, child := range root.Children {
		switch child.Tag {
		case "Filter":
			f := vnodeToFilter(child)
			d.Filters = append(d.Filters, f)

		case "Row":
			row := vnodeToRow(child)
			if len(row.Widgets) > 0 {
				d.Rows = append(d.Rows, row)
			}

		case "Tabs":
			// <Tabs> contains <Tab> children, each with a name and rows.
			for _, tabChild := range child.Children {
				if tabChild.Tag != "Tab" {
					continue
				}
				tabName := asString(tabChild.Props["name"])
				for _, rowChild := range tabChild.Children {
					if rowChild.Tag == "Row" {
						row := vnodeToRow(rowChild)
						row.Tab = tabName
						if len(row.Widgets) > 0 {
							d.Rows = append(d.Rows, row)
						}
					} else {
						// Widget directly inside a Tab — wrap in a Row.
						w := vnodeToWidget(rowChild)
						d.Rows = append(d.Rows, Row{Tab: tabName, Widgets: []Widget{w}})
					}
				}
			}

		case "Query":
			if d.Queries == nil {
				d.Queries = make(map[string]Query)
			}
			name := asString(child.Props["name"])
			q := Query{
				SQL:        asString(child.Props["sql"]),
				Connection: asString(child.Props["connection"]),
				Model:      asString(child.Props["model"]),
				Dimensions: asSemanticDimensionRefs(child.Props["dimensions"]),
				Metrics:    asStringSlice(child.Props["metrics"]),
				Filters:    asSemanticFilters(child.Props["filters"]),
				Segments:   asStringSlice(child.Props["segments"]),
				Sort:       asSemanticSorts(child.Props["sort"]),
				Limit:      asInt(child.Props["limit"]),
			}
			d.Queries[name] = q

		default:
			// A widget at the top level (outside a Row) — wrap in its own Row.
			w := vnodeToWidget(child)
			d.Rows = append(d.Rows, Row{Widgets: []Widget{w}})
		}
	}

	return d, nil
}

func vnodeToFilter(n *vnode) Filter {
	f := Filter{
		Name:     asString(n.Props["name"]),
		Type:     asString(n.Props["type"]),
		Multiple: asBool(n.Props["multiple"]),
		Default:  n.Props["default"],
	}

	if opts, ok := n.Props["options"]; ok {
		if m, ok := opts.(map[string]interface{}); ok {
			fo := &FilterOptions{}
			if vals, ok := m["values"]; ok {
				fo.Values = asStringSlice(vals)
			}
			if q, ok := m["query"]; ok {
				fo.Query = asString(q)
			}
			if c, ok := m["connection"]; ok {
				fo.Connection = asString(c)
			}
			if p, ok := m["presets"]; ok {
				fo.Presets = asStringSlice(p)
			}
			f.Options = fo
		}
	}

	return f
}

func vnodeToRow(n *vnode) Row {
	row := Row{
		Height: n.Props["height"],
	}
	for _, child := range n.Children {
		w := vnodeToWidget(child)
		row.Widgets = append(row.Widgets, w)
	}
	return row
}

func vnodeToWidget(n *vnode) Widget {
	w := Widget{
		ID:          asString(n.Props["id"]),
		Name:        asString(n.Props["name"]),
		Description: asString(n.Props["description"]),
		Type:        widgetType(n.Tag),
		Col:         asInt(n.Props["col"]),

		// Query source
		QueryRef:   asString(n.Props["query"]),
		SQL:        asString(n.Props["sql"]),
		MetricRef:  asString(n.Props["metric"]),
		Model:      asString(n.Props["model"]),
		Connection: asString(n.Props["connection"]),

		// Declarative chart fields
		Dimension:   asString(n.Props["dimension"]),
		Granularity: asString(n.Props["granularity"]),
		Dimensions:  asSemanticDimensionRefs(n.Props["dimensions"]),
		MetricRefs:  asStringSlice(n.Props["metrics"]),
		Filters:     asSemanticFilters(n.Props["filters"]),
		Segments:    asStringSlice(n.Props["segments"]),
		Sort:        asSemanticSorts(n.Props["sort"]),
		Limit:       asInt(n.Props["limit"]),

		// Chart fields
		Chart:      asString(n.Props["chart"]),
		X:          asAxisEncoding(n.Props["x"]),
		Y:          asAxisEncoding(n.Props["y"]),
		Y2:         asAxisEncoding(n.Props["y2"]),
		Label:      asString(n.Props["label"]),
		Value:      asValueEncoding(n.Props["value"]),
		Color:      asColorEncoding(n.Props["color"]),
		Stacked:    asBool(n.Props["stacked"]),
		Normalized: asBool(n.Props["normalized"]),
		Horizontal: asBoolPtr(n.Props["horizontal"]),
		Size:       asString(n.Props["size"]),
		Source:     asString(n.Props["source"]),
		Target:     asString(n.Props["target"]),
		Bins:       asInt(n.Props["bins"]),
		ShowValues: asBool(n.Props["showValues"]),
		Lines:      asStringSlice(n.Props["lines"]),
		Series:     asSeriesStyles(n.Props["series"]),
		YMin:       asBoundEncoding(n.Props["yMin"]),
		YMax:       asBoundEncoding(n.Props["yMax"]),
		RefLines:   asRefLines(n.Props["refLines"]),
		RefBands:   asRefBands(n.Props["refBands"]),

		// Table fields
		Columns: asTableColumns(n.Props["columns"]),

		// Text fields
		Content: asString(n.Props["content"]),

		// Image fields
		Src: asString(n.Props["src"]),
		Alt: asString(n.Props["alt"]),
	}

	return w
}

// widgetType maps a JSX tag name to a widget type string.
func widgetType(tag string) string {
	switch tag {
	case "Metric":
		return WidgetTypeMetric
	case "Chart":
		return WidgetTypeChart
	case "Table":
		return WidgetTypeTable
	case "Text":
		return WidgetTypeText
	case "Divider":
		return WidgetTypeDivider
	case "Image":
		return WidgetTypeImage
	default:
		return strings.ToLower(tag)
	}
}

// Type conversion helpers.

func asString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// asBoundEncoding reads a yMin/yMax prop: a column-name string, or a per-series
// map {series: column}. Returns nil when empty so omitempty drops it.
func asBoundEncoding(v interface{}) *BoundEncoding {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return &BoundEncoding{Field: t}
	case map[string]interface{}:
		m := map[string]string{}
		for k, vv := range t {
			if s, ok := vv.(string); ok && s != "" {
				m[k] = s
			}
		}
		if len(m) == 0 {
			return nil
		}
		return &BoundEncoding{Map: m}
	default:
		return nil
	}
}

func asInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}

func asBool(v interface{}) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// asBoolPtr returns nil when the prop is absent so a distinct absent-vs-false can
// survive (forest defaults to horizontal; an explicit false must not be dropped).
func asBoolPtr(v interface{}) *bool {
	if v == nil {
		return nil
	}
	if b, ok := v.(bool); ok {
		return &b
	}
	return nil
}

func asFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func asRefLines(v interface{}) []RefLine {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]RefLine, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, RefLine{
			Axis:  asString(m["axis"]),
			Value: asFloat(m["value"]),
			Label: asString(m["label"]),
			Color: asString(m["color"]),
		})
	}
	return out
}

func asRefBands(v interface{}) []RefBand {
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]RefBand, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, RefBand{
			Axis:  asString(m["axis"]),
			From:  asFloat(m["from"]),
			To:    asFloat(m["to"]),
			Label: asString(m["label"]),
			Color: asString(m["color"]),
		})
	}
	return out
}

func asStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = asString(item)
		}
		return result
	case []string:
		return val
	default:
		return nil
	}
}

func asAxisEncoding(v interface{}) *AxisEncoding {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return &AxisEncoding{Field: val}
	case []interface{}:
		list := asStringSlice(val)
		if len(list) == 0 {
			return nil
		}
		return &AxisEncoding{Field: list}
	case []string:
		if len(val) == 0 {
			return nil
		}
		return &AxisEncoding{Field: val}
	case map[string]interface{}:
		a := &AxisEncoding{
			Type:   asString(val["type"]),
			Title:  asString(val["title"]),
			Format: asString(val["format"]),
			Curve:  asString(val["curve"]),
			Dash:   asString(val["dash"]),
		}
		if b, ok := val["beginAtZero"].(bool); ok {
			a.BeginAtZero = &b
		}
		if m, ok := val["markers"].(bool); ok {
			a.Markers = &m
		}
		switch f := val["field"].(type) {
		case string:
			if f != "" {
				a.Field = f
			}
		case []interface{}:
			list := asStringSlice(f)
			if len(list) > 0 {
				a.Field = list
			}
		case []string:
			if len(f) > 0 {
				a.Field = f
			}
		}
		if a.Field == nil && a.Type == "" && a.Title == "" && a.Format == "" &&
			a.Curve == "" && a.Dash == "" && a.BeginAtZero == nil && a.Markers == nil {
			return nil
		}
		return a
	default:
		return nil
	}
}

// asSeriesStyles reads the per-series style map ({column: {color, curve, dash}})
// from a TSX/JSX dashboard's widget-level series prop.
func asSeriesStyles(v interface{}) map[string]SeriesStyle {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]SeriesStyle, len(m))
	for k, item := range m {
		sm, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out[k] = SeriesStyle{
			Color: asString(sm["color"]),
			Curve: asString(sm["curve"]),
			Dash:  asString(sm["dash"]),
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func asValueEncoding(v interface{}) *ValueEncoding {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return &ValueEncoding{Field: val}
	case map[string]interface{}:
		field := asString(val["field"])
		if field == "" {
			return nil
		}
		return &ValueEncoding{
			Field:  field,
			Type:   asString(val["type"]),
			Format: asString(val["format"]),
		}
	default:
		return nil
	}
}

func asColorEncoding(v interface{}) *ColorEncoding {
	if v == nil {
		return nil
	}
	val, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	field := asString(val["field"])
	if field == "" {
		return nil
	}
	return &ColorEncoding{Field: field}
}

func asStringMap(v interface{}) map[string]string {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for key, value := range m {
		out[key] = asString(value)
	}
	return out
}

func asSemanticDimensionRefs(v interface{}) []SemanticDimensionRef {
	if v == nil {
		return nil
	}
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]SemanticDimensionRef, 0, len(raw))
	for _, item := range raw {
		switch val := item.(type) {
		case string:
			out = append(out, SemanticDimensionRef{Name: val})
		case map[string]interface{}:
			out = append(out, SemanticDimensionRef{
				Name:        asString(val["name"]),
				Granularity: asString(val["granularity"]),
			})
		}
	}
	return out
}

func asSemanticFilters(v interface{}) []SemanticQueryFilter {
	if v == nil {
		return nil
	}
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]SemanticQueryFilter, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, SemanticQueryFilter{
			Dimension:  asString(m["dimension"]),
			Operator:   asString(m["operator"]),
			Value:      m["value"],
			Expression: asString(m["expression"]),
		})
	}
	return out
}

func asSemanticSorts(v interface{}) []SemanticSort {
	if v == nil {
		return nil
	}
	raw, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]SemanticSort, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, SemanticSort{
			Name:      asString(m["name"]),
			Direction: asString(m["direction"]),
		})
	}
	return out
}

func asMap(v interface{}) map[string]interface{} {
	if v == nil {
		return map[string]interface{}{}
	}
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func asTableColumns(v interface{}) []TableColumn {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var cols []TableColumn
	for _, item := range arr {
		if m, ok := item.(map[string]interface{}); ok {
			col := TableColumn{
				Name:   asString(m["name"]),
				Label:  asString(m["label"]),
				Number: asString(m["number"]),
				Align:  asString(m["align"]),
				Like:   asString(m["like"]),
				Hidden: asBool(m["hidden"]),
			}
			// `format` is polymorphic: a string is the legacy value-display
			// shorthand (== `number`), a list is the style layers.
			if s, ok := m["format"].(string); ok {
				if col.Number == "" {
					col.Number = s
				}
			} else {
				col.Format = asFormatLayers(m["format"])
			}
			cols = append(cols, col)
		}
	}
	return cols
}

// asFormatLayers reads a column's ordered `format` list of style layers.
func asFormatLayers(v interface{}) []FormatLayer {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	var layers []FormatLayer
	for _, item := range arr {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		layers = append(layers, FormatLayer{
			If:              asString(m["if"]),
			Value:           m["value"],
			BackgroundColor: m["backgroundColor"],
			Range:           asFloatSlice(m["range"]),
			Unit:            asString(m["unit"]),
			TextColor:       asString(m["textColor"]),
			Bold:            asBool(m["bold"]),
			Italic:          asBool(m["italic"]),
			Underline:       asBool(m["underline"]),
			Strikethrough:   asBool(m["strikethrough"]),
		})
	}
	return layers
}

func asFloatSlice(v interface{}) []float64 {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, item := range arr {
		switch n := item.(type) {
		case float64:
			out = append(out, n)
		case int:
			out = append(out, float64(n))
		case int64:
			out = append(out, float64(n))
		}
	}
	return out
}

// IsTSXFile checks if a filename matches the .dashboard.tsx convention.
func IsTSXFile(name string) bool {
	return strings.HasSuffix(name, ".dashboard.tsx")
}
