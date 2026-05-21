package dashboard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/bruin-data/dac/schemas"
)

// LoadDir discovers and loads all dashboard files from the project's dashboards directory.
func LoadDir(dir string, opts ...TSXOption) ([]*Dashboard, error) {
	paths := ResolveProjectPaths(dir)
	semanticModels, err := loadSemanticModels(paths)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(paths.DashboardDir)
	if err != nil {
		return nil, fmt.Errorf("reading dashboard directory: %w", err)
	}

	var dashboards []*Dashboard
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		path := filepath.Join(paths.DashboardDir, entry.Name())
		name := entry.Name()

		switch {
		case isYAMLFile(name):
			d, err := loadFileWithContext(path, paths, semanticModels)
			if err != nil {
				return nil, fmt.Errorf("loading %s: %w", name, err)
			}
			dashboards = append(dashboards, d)

		case IsTSXFile(name):
			d, err := loadTSXFileWithContext(path, paths, semanticModels, opts...)
			if err != nil {
				return nil, fmt.Errorf("loading %s: %w", name, err)
			}
			dashboards = append(dashboards, d)
		}
	}

	return dashboards, nil
}

// LoadOneByName finds a dashboard by name using a two-pass approach:
// first a cheap metadata scan (no query execution) to find the file path,
// then a full load of just that one file. Returns nil, nil if not found.
func LoadOneByName(dir, name string, opts ...TSXOption) (*Dashboard, error) {
	// Pass 1: find the file path without executing queries.
	metaDashboards, err := LoadDir(dir) // no opts = no query backend
	if err != nil {
		return nil, err
	}

	var filePath string
	for _, d := range metaDashboards {
		if d.Name == name {
			filePath = d.FilePath
			break
		}
	}
	if filePath == "" {
		return nil, nil
	}

	paths := ResolveProjectPaths(dir)
	semanticModels, err := loadSemanticModels(paths)
	if err != nil {
		return nil, err
	}

	// Pass 2: load just that file with query execution if needed.
	if IsTSXFile(filepath.Base(filePath)) {
		return loadTSXFileWithContext(filePath, paths, semanticModels, opts...)
	}
	return loadFileWithContext(filePath, paths, semanticModels)
}

// LoadFile loads a single dashboard YAML file.
func LoadFile(path string) (*Dashboard, error) {
	paths := ResolveProjectPathsForFile(path)
	semanticModels, err := loadSemanticModels(paths)
	if err != nil {
		return nil, err
	}
	return loadFileWithContext(path, paths, semanticModels)
}

func loadFileWithContext(path string, paths ProjectPaths, semanticModels semanticModelSet) (*Dashboard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}
	if err := schemas.ValidateYAML(schemas.DashboardV1ID, data); err != nil {
		return nil, err
	}

	var d Dashboard
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	if d.Schema == "" {
		d.Schema = schemas.DashboardV1ID
	}

	d.FilePath = path
	d.FileType = "yaml"
	d.SetProjectContext(paths.RootDir, semanticModels.models, semanticModels.invalid)

	// Resolve file-based queries (both named and inline on widgets).
	dir := filepath.Dir(path)
	if err := resolveQueryFiles(&d, dir); err != nil {
		return nil, err
	}

	postProcessDashboard(&d)

	return &d, nil
}

// resolveQueryFiles reads external .sql files referenced by queries and widgets.
func resolveQueryFiles(d *Dashboard, baseDir string) error {
	// Resolve named queries with file references.
	for name, q := range d.Queries {
		if q.File != "" {
			sql, err := readQueryFile(baseDir, q.File)
			if err != nil {
				return fmt.Errorf("query %q: %w", name, err)
			}
			q.SQL = sql
			d.Queries[name] = q
		}
	}

	// Resolve inline file references on widgets.
	for i, row := range d.Rows {
		for j, w := range row.Widgets {
			if w.File != "" && w.QueryRef == "" {
				sql, err := readQueryFile(baseDir, w.File)
				if err != nil {
					return fmt.Errorf("widget %q: %w", w.Name, err)
				}
				d.Rows[i].Widgets[j].SQL = sql
			}
		}
	}

	return nil
}

func readQueryFile(baseDir, relPath string) (string, error) {
	path := filepath.Join(baseDir, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading query file %s: %w", relPath, err)
	}
	return string(data), nil
}

func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yml" || ext == ".yaml"
}

func postProcessDashboard(d *Dashboard) {
	for i, row := range d.Rows {
		for j, w := range row.Widgets {
			if w.Dimension != "" && len(w.MetricRefs) > 0 {
				if x := defaultSemanticDimensionAlias(d, &w); x != "" && w.XField() == "" {
					d.Rows[i].Widgets[j].X = newAxisField(x)
				}
				if len(w.YFields()) == 0 {
					d.Rows[i].Widgets[j].Y = newAxisFields(w.MetricRefs)
				}
			}

			if w.QueryRef == "" || w.XField() != "" || len(w.YFields()) > 0 {
				continue
			}
			q, ok := d.Queries[w.QueryRef]
			if !ok || !q.IsSemantic() {
				continue
			}
			if len(q.Dimensions) == 1 {
				d.Rows[i].Widgets[j].X = newAxisField(q.Dimensions[0].Name)
			}
			if len(q.Metrics) > 0 {
				d.Rows[i].Widgets[j].Y = newAxisFields(append([]string(nil), q.Metrics...))
			}
		}
	}
}

func defaultSemanticDimensionAlias(d *Dashboard, w *Widget) string {
	if w.Model != "" || d.Model != "" {
		return w.Dimension
	}
	if dims := d.SemanticDimensions(); dims != nil {
		if dim, ok := dims[w.Dimension]; ok {
			return DimensionAlias(dim.Column)
		}
	}
	return ""
}
