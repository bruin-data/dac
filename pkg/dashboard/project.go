package dashboard

import (
	"os"
	"path/filepath"

	sem "github.com/bruin-data/bruin/semantic-engine"
)

type semanticModelSet struct {
	models  map[string]*sem.Model
	invalid map[string]error
}

type ProjectPaths struct {
	RootDir      string
	DashboardDir string
	SemanticDir  string
	ThemesDir    string
}

func ResolveProjectPaths(dir string) ProjectPaths {
	clean := filepath.Clean(dir)
	paths := ProjectPaths{
		RootDir:      clean,
		DashboardDir: clean,
	}

	projectDashboards := filepath.Join(clean, "dashboards")
	if isDir(projectDashboards) {
		paths.DashboardDir = projectDashboards
	} else if filepath.Base(clean) == "dashboards" {
		parent := filepath.Dir(clean)
		if isDir(filepath.Join(parent, "semantic")) || isDir(filepath.Join(parent, "themes")) {
			paths.RootDir = parent
		}
	}

	rootSemantic := filepath.Join(paths.RootDir, "semantic")
	if isDir(rootSemantic) {
		paths.SemanticDir = rootSemantic
	}

	rootThemes := filepath.Join(paths.RootDir, "themes")
	dashboardThemes := filepath.Join(paths.DashboardDir, "themes")
	switch {
	case isDir(rootThemes):
		paths.ThemesDir = rootThemes
	case isDir(dashboardThemes):
		paths.ThemesDir = dashboardThemes
	}

	return paths
}

func ResolveProjectPathsForFile(path string) ProjectPaths {
	return ResolveProjectPaths(filepath.Dir(path))
}

func loadSemanticModels(paths ProjectPaths) (semanticModelSet, error) {
	if paths.SemanticDir == "" {
		return semanticModelSet{
			models:  map[string]*sem.Model{},
			invalid: map[string]error{},
		}, nil
	}
	models, invalid, err := sem.LoadDirPartial(paths.SemanticDir)
	if err != nil {
		return semanticModelSet{}, err
	}
	return semanticModelSet{
		models:  models,
		invalid: invalid,
	}, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
