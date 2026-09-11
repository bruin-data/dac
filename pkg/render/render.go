package render

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/server"
	"github.com/bruin-data/dac/pkg/theme"
)

// Config holds the configuration for a static build.
type Config struct {
	DashboardDir string
	Dashboard    string // dashboard name
	OutputDir    string
	Filters      map[string]any // filter overrides (merged over defaults)
	TemplateName string
	ConfigFile   string
	Environment  string
	Frontend     fs.FS // embedded frontend FS
}

// Build produces a self-contained static directory for a single dashboard.
// The output contains the React SPA with query results baked into index.html
// via a window.__DAC_STATIC__ payload.
func Build(ctx context.Context, cfg Config) error {
	paths := dashboard.ResolveProjectPaths(cfg.DashboardDir)

	// Load dashboards.
	dashboards, err := dashboard.LoadDir(cfg.DashboardDir)
	if err != nil {
		return fmt.Errorf("loading dashboards: %w", err)
	}
	if err := dashboard.ValidateAll(dashboards); err != nil {
		return fmt.Errorf("validating dashboards: %w", err)
	}

	// Find the target dashboard.
	var d *dashboard.Dashboard
	for _, dash := range dashboards {
		if dash.Name == cfg.Dashboard {
			d = dash
			break
		}
	}
	if d == nil {
		return fmt.Errorf("dashboard not found: %q", cfg.Dashboard)
	}

	// Create query backend.
	backend := &query.BruinCLIBackend{
		ConfigFile:  cfg.ConfigFile,
		Environment: cfg.Environment,
	}

	// Set up theme registry.
	themes := theme.NewRegistry()
	templateName := cfg.TemplateName
	if strings.HasSuffix(templateName, ".yml") || strings.HasSuffix(templateName, ".yaml") {
		t, err := theme.LoadFile(templateName)
		if err != nil {
			return fmt.Errorf("loading template file: %w", err)
		}
		themes.Add(t)
		templateName = t.Name
	}
	if paths.ThemesDir != "" {
		if err := themes.LoadUserThemes(paths.ThemesDir); err != nil {
			log.Printf("Warning: could not load user themes: %v", err)
		}
	}

	// Build config payload.
	configPayload := map[string]any{
		"template":      templateName,
		"admin_enabled": false,
	}
	if t, ok := themes.Get(templateName); ok {
		configPayload["tokens"] = t.Tokens
	}

	// Merge filters over dashboard defaults.
	filters := d.DefaultFilters()
	for k, v := range cfg.Filters {
		filters[k] = v
	}

	// Resolve widget jobs.
	jobs, err := server.ResolveWidgetJobs(d, filters)
	if err != nil {
		return fmt.Errorf("resolving widget jobs: %w", err)
	}

	// Execute all jobs concurrently.
	results := make(map[string]*server.WidgetQueryResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, 8)
	for _, j := range jobs {
		wg.Add(1)
		go func(j server.WidgetJob) {
			defer wg.Done()
			sem <- struct{}{}
			wr := server.ExecuteWidgetQuery(ctx, backend, j)
			<-sem
			mu.Lock()
			results[j.ID] = wr
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	// Build dashboard summaries.
	summaries := make([]server.DashboardSummary, 0, len(dashboards))
	for _, dash := range dashboards {
		summaries = append(summaries, server.MakeDashboardSummary(dash))
	}

	// Build the static payload.
	payload := map[string]any{
		"config":     configPayload,
		"dashboard":  d,
		"dashboards": summaries,
		"widgetData": results,
		"filters":    filters,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling static payload: %w", err)
	}

	// Read index.html from the frontend FS.
	indexBytes, err := fs.ReadFile(cfg.Frontend, "index.html")
	if err != nil {
		return fmt.Errorf("reading index.html from frontend: %w", err)
	}

	// Inject the payload before </head>.
	scriptTag := fmt.Sprintf("<script>window.__DAC_STATIC__=%s;</script>", payloadJSON)
	modifiedIndex := strings.Replace(string(indexBytes), "</head>", scriptTag+"</head>", 1)

	if err := writeStaticOutput(cfg.OutputDir, cfg.Frontend, modifiedIndex); err != nil {
		return err
	}

	return nil
}

// writeStaticOutput copies the whole frontend tree (not just index.html's direct
// references) so lazy chunks like vega-embed are included, clearing stale
// content-hashed chunks from prior rebuilds first.
func writeStaticOutput(outputDir string, frontend fs.FS, indexHTML string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(outputDir, "assets")); err != nil {
		return fmt.Errorf("clearing stale assets: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return fmt.Errorf("writing index.html: %w", err)
	}

	err := fs.WalkDir(frontend, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == "index.html" {
			return nil
		}
		data, err := fs.ReadFile(frontend, path)
		if err != nil {
			return fmt.Errorf("reading asset %s: %w", path, err)
		}
		outPath := filepath.Join(outputDir, path)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", path, err)
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("copying frontend assets: %w", err)
	}

	return nil
}
