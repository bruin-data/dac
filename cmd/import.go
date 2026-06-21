package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	mbimport "github.com/bruin-data/dac/pkg/importer/metabase"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

func importCmd() *cli.Command {
	return &cli.Command{
		Name:  "import",
		Usage: "Import dashboards from external tools",
		Commands: []*cli.Command{
			importMetabaseCmd(),
		},
	}
}

func importMetabaseCmd() *cli.Command {
	return &cli.Command{
		Name:  "metabase",
		Usage: "Convert a Metabase dashboard to DAC YAML",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "input",
				Aliases: []string{"i"},
				Usage:   "Path to a Metabase dashboard API response JSON/YAML file",
			},
			&cli.StringFlag{
				Name:  "url",
				Usage: "Metabase base URL for live import",
			},
			&cli.StringFlag{
				Name:  "dashboard-id",
				Usage: "Metabase dashboard ID for live import",
			},
			&cli.StringFlag{
				Name:    "api-key",
				Usage:   "Metabase API key (or set METABASE_API_KEY)",
				Sources: cli.EnvVars("METABASE_API_KEY"),
			},
			&cli.StringFlag{
				Name:    "session-token",
				Usage:   "Metabase session token (or set METABASE_SESSION_TOKEN)",
				Sources: cli.EnvVars("METABASE_SESSION_TOKEN"),
			},
			&cli.StringFlag{
				Name:    "connection",
				Aliases: []string{"c"},
				Usage:   "DAC connection name to set on the generated dashboard",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output YAML path (default: <dir>/<dashboard-slug>.yml; use - for stdout)",
			},
			dirFlag,
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Overwrite an existing output file",
			},
			&cli.BoolFlag{
				Name:  "strict",
				Usage: "Fail instead of creating placeholders for unsupported Metabase cards",
			},
			&cli.BoolFlag{
				Name:  "semantic",
				Usage: "Generate DAC semantic models for explicit Metabase models/metrics and convert eligible widgets to semantic references",
			},
			&cli.BoolFlag{
				Name:  "semantic-strict",
				Usage: "Fail when semantic import cannot represent a Metabase model-backed card semantically",
			},
			&cli.StringFlag{
				Name:  "semantic-dir",
				Usage: "Directory for generated semantic model files (default: sibling semantic/ next to dashboards/)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			data, source, err := readMetabaseInput(ctx, cmd)
			if err != nil {
				return err
			}

			semantic := cmd.Bool("semantic") || cmd.Bool("semantic-strict")
			if semantic && cmd.String("output") == "-" {
				return fmt.Errorf("--semantic cannot be used with --output - because generated semantic model files need filesystem paths")
			}

			project, report, err := mbimport.ConvertProject(data, mbimport.Options{
				Connection:     cmd.String("connection"),
				Strict:         cmd.Bool("strict"),
				Semantic:       semantic,
				SemanticStrict: cmd.Bool("semantic-strict"),
			})
			if err != nil {
				return err
			}
			d := project.Dashboard

			out, err := yaml.Marshal(d)
			if err != nil {
				return fmt.Errorf("encoding DAC YAML: %w", err)
			}

			outputPath := cmd.String("output")
			if outputPath == "" {
				outputPath = filepath.Join(cmd.String("dir"), slugifyFilename(d.Name)+".yml")
			}
			if err := writeImportOutput(outputPath, out, cmd.Bool("force")); err != nil {
				return err
			}

			var semanticPaths []string
			if semantic {
				semanticDir := cmd.String("semantic-dir")
				if semanticDir == "" {
					semanticDir = defaultSemanticDir(cmd.String("dir"), outputPath)
				}
				for _, file := range project.SemanticModels {
					data, err := yaml.Marshal(file.Model)
					if err != nil {
						return fmt.Errorf("encoding DAC semantic model %q: %w", file.Model.Name, err)
					}
					path := filepath.Join(semanticDir, file.Filename)
					if err := writeImportOutput(path, data, cmd.Bool("force")); err != nil {
						return err
					}
					semanticPaths = append(semanticPaths, path)
				}
			}

			for _, warning := range report.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", warning)
			}
			if outputPath == "-" {
				fmt.Fprintf(os.Stderr, "Imported %q from %s: %d widget(s), %d SQL-backed, %d semantic, %d placeholder(s).\n",
					d.Name, source, report.WidgetCount, report.SQLWidgetCount, report.SemanticWidgetCount, report.PlaceholderCount)
			} else {
				if len(semanticPaths) > 0 {
					fmt.Printf("Imported %q from %s to %s and %d semantic model file(s) (%d widget(s), %d SQL-backed, %d semantic, %d placeholder(s)).\n",
						d.Name, source, outputPath, len(semanticPaths), report.WidgetCount, report.SQLWidgetCount, report.SemanticWidgetCount, report.PlaceholderCount)
				} else {
					fmt.Printf("Imported %q from %s to %s (%d widget(s), %d SQL-backed, %d semantic, %d placeholder(s)).\n",
						d.Name, source, outputPath, report.WidgetCount, report.SQLWidgetCount, report.SemanticWidgetCount, report.PlaceholderCount)
				}
			}
			return nil
		},
	}
}

func readMetabaseInput(ctx context.Context, cmd *cli.Command) ([]byte, string, error) {
	input := cmd.String("input")
	baseURL := cmd.String("url")
	dashboardID := cmd.String("dashboard-id")

	switch {
	case input != "" && (baseURL != "" || dashboardID != ""):
		return nil, "", fmt.Errorf("use either --input or --url/--dashboard-id, not both")
	case input != "":
		data, err := os.ReadFile(input)
		if err != nil {
			return nil, "", fmt.Errorf("reading Metabase input: %w", err)
		}
		return data, input, nil
	case baseURL != "" || dashboardID != "":
		if baseURL == "" || dashboardID == "" {
			return nil, "", fmt.Errorf("live import requires both --url and --dashboard-id")
		}
		data, err := fetchMetabaseDashboard(ctx, baseURL, dashboardID, cmd.String("api-key"), cmd.String("session-token"))
		if err != nil {
			return nil, "", err
		}
		return data, strings.TrimRight(baseURL, "/") + "/api/dashboard/" + dashboardID, nil
	default:
		return nil, "", fmt.Errorf("provide --input or --url/--dashboard-id")
	}
}

func fetchMetabaseDashboard(ctx context.Context, baseURL, dashboardID, apiKey, sessionToken string) ([]byte, error) {
	if apiKey == "" && sessionToken == "" {
		return nil, fmt.Errorf("live import requires --api-key, --session-token, METABASE_API_KEY, or METABASE_SESSION_TOKEN")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Metabase URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/dashboard/" + url.PathEscape(dashboardID)

	client := http.DefaultClient
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating Metabase request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if sessionToken != "" {
		req.Header.Set("X-Metabase-Session", sessionToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Metabase dashboard: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Metabase response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("Metabase API returned %s: %s", resp.Status, msg)
	}
	return hydrateMetabaseSourceCards(ctx, client, u, body, apiKey, sessionToken)
}

func hydrateMetabaseSourceCards(ctx context.Context, client *http.Client, dashboardURL *url.URL, dashboardBody []byte, apiKey, sessionToken string) ([]byte, error) {
	var root map[string]any
	if err := json.Unmarshal(dashboardBody, &root); err != nil {
		return dashboardBody, nil
	}

	sourceCards := map[string]any{}
	metrics := map[string]any{}
	tables := map[string]any{}
	seenSourceCards := map[string]bool{}
	seenMetrics := map[string]bool{}
	seenTables := map[string]bool{}
	for {
		ids := collectMetabaseSourceCardIDs(root)
		var fetched bool
		for id := range ids {
			if seenSourceCards[id] {
				continue
			}
			seenSourceCards[id] = true
			card, err := fetchMetabaseCard(ctx, client, dashboardURL, id, apiKey, sessionToken)
			if err != nil {
				return nil, err
			}
			sourceCards[id] = card
			root["x-dac-metabase-source-cards"] = sourceCards
			fetched = true
		}

		metricIDs := collectMetabaseMetricIDs(root)
		for id := range metricIDs {
			if seenMetrics[id] {
				continue
			}
			seenMetrics[id] = true
			metric, err := fetchMetabaseMetric(ctx, client, dashboardURL, id, apiKey, sessionToken)
			if err != nil {
				return nil, err
			}
			metrics[id] = metric
			root["x-dac-metabase-metrics"] = metrics
			fetched = true
		}

		tableIDs := collectMetabaseTableIDs(root)
		for id := range tableIDs {
			if seenTables[id] {
				continue
			}
			seenTables[id] = true
			table, err := fetchMetabaseTableMetadata(ctx, client, dashboardURL, id, apiKey, sessionToken)
			if err != nil {
				return nil, err
			}
			tables[id] = table
			root["x-dac-metabase-tables"] = tables
			fetched = true
		}

		if !fetched {
			break
		}
	}
	if len(sourceCards) == 0 && len(metrics) == 0 && len(tables) == 0 {
		return dashboardBody, nil
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("encoding hydrated Metabase dashboard: %w", err)
	}
	return out, nil
}

func fetchMetabaseCard(ctx context.Context, client *http.Client, dashboardURL *url.URL, id, apiKey, sessionToken string) (map[string]any, error) {
	u := *dashboardURL
	u.RawQuery = ""
	prefix := u.Path
	if idx := strings.Index(prefix, "/api/dashboard/"); idx >= 0 {
		prefix = prefix[:idx]
	}
	u.Path = strings.TrimRight(prefix, "/") + "/api/card/" + url.PathEscape(strings.TrimPrefix(id, "card__"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating Metabase card request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if sessionToken != "" {
		req.Header.Set("X-Metabase-Session", sessionToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Metabase source card %s: %w", id, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Metabase source card %s: %w", id, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("Metabase API returned %s for source card %s: %s", resp.Status, id, msg)
	}
	var card map[string]any
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("parsing Metabase source card %s: %w", id, err)
	}
	return card, nil
}

func fetchMetabaseMetric(ctx context.Context, client *http.Client, dashboardURL *url.URL, id, apiKey, sessionToken string) (map[string]any, error) {
	u := *dashboardURL
	u.RawQuery = ""
	prefix := u.Path
	if idx := strings.Index(prefix, "/api/dashboard/"); idx >= 0 {
		prefix = prefix[:idx]
	}
	u.Path = strings.TrimRight(prefix, "/") + "/api/metric/" + url.PathEscape(strings.TrimPrefix(id, "metric__"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating Metabase metric request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if sessionToken != "" {
		req.Header.Set("X-Metabase-Session", sessionToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Metabase metric %s: %w", id, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Metabase metric %s: %w", id, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("Metabase API returned %s for metric %s: %s", resp.Status, id, msg)
	}
	var metric map[string]any
	if err := json.Unmarshal(body, &metric); err != nil {
		return nil, fmt.Errorf("parsing Metabase metric %s: %w", id, err)
	}
	return metric, nil
}

func fetchMetabaseTableMetadata(ctx context.Context, client *http.Client, dashboardURL *url.URL, id, apiKey, sessionToken string) (map[string]any, error) {
	u := *dashboardURL
	u.RawQuery = ""
	prefix := u.Path
	if idx := strings.Index(prefix, "/api/dashboard/"); idx >= 0 {
		prefix = prefix[:idx]
	}
	u.Path = strings.TrimRight(prefix, "/") + "/api/table/" + url.PathEscape(id) + "/query_metadata"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating Metabase table metadata request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if sessionToken != "" {
		req.Header.Set("X-Metabase-Session", sessionToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Metabase table %s metadata: %w", id, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("reading Metabase table %s metadata: %w", id, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("Metabase API returned %s for table %s metadata: %s", resp.Status, id, msg)
	}
	var table map[string]any
	if err := json.Unmarshal(body, &table); err != nil {
		return nil, fmt.Errorf("parsing Metabase table %s metadata: %w", id, err)
	}
	return table, nil
}

func collectMetabaseSourceCardIDs(v any) map[string]bool {
	ids := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				switch key {
				case "source-card", "source_card":
					if id := metabaseSourceCardID(value); id != "" {
						ids[id] = true
					}
				case "source-table", "source_table":
					if id := metabaseSourceTableCardID(value); id != "" {
						ids[id] = true
					}
				}
				walk(value)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(v)
	return ids
}

func metabaseSourceCardID(v any) string {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "card__") {
			return strings.TrimPrefix(x, "card__")
		}
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case json.Number:
		return x.String()
	}
	return ""
}

func metabaseSourceTableCardID(v any) string {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "card__") {
			return strings.TrimPrefix(x, "card__")
		}
	}
	return ""
}

func collectMetabaseTableIDs(v any) map[string]bool {
	ids := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				switch key {
				case "source-table", "source_table":
					if id := metabasePhysicalTableID(value); id != "" {
						ids[id] = true
					}
				}
				walk(value)
			}
		case []any:
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(v)
	return ids
}

func metabasePhysicalTableID(v any) string {
	switch x := v.(type) {
	case string:
		if strings.HasPrefix(x, "card__") {
			return ""
		}
		return strings.TrimSpace(x)
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case json.Number:
		return x.String()
	default:
		return ""
	}
}

func collectMetabaseMetricIDs(v any) map[string]bool {
	ids := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for key, value := range x {
				switch key {
				case "metric-id", "metric_id":
					if id := metabaseMetricID(value); id != "" {
						ids[id] = true
					}
				}
				walk(value)
			}
		case []any:
			if id := metabaseMetricAggregationID(x); id != "" {
				ids[id] = true
			}
			for _, item := range x {
				walk(item)
			}
		}
	}
	walk(v)
	return ids
}

func metabaseMetricAggregationID(parts []any) string {
	if len(parts) == 0 {
		return ""
	}
	kind, _ := parts[0].(string)
	if kind != "metric" {
		return ""
	}
	for _, part := range parts[1:] {
		if id := metabaseMetricID(part); id != "" {
			return id
		}
	}
	return ""
}

func metabaseMetricID(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimPrefix(x, "metric__")
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case json.Number:
		return x.String()
	case map[string]any:
		for _, key := range []string{"metric-id", "metric_id", "id"} {
			if id := metabaseMetricID(x[key]); id != "" {
				return id
			}
		}
	}
	return ""
}

func writeImportOutput(path string, data []byte, force bool) error {
	if path == "-" {
		_, err := os.Stdout.Write(data)
		return err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; pass --force to overwrite", path)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("checking output path: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing import output: %w", err)
	}
	return nil
}

func defaultSemanticDir(dashboardDir, outputPath string) string {
	baseDir := filepath.Clean(dashboardDir)
	if outputPath != "" && outputPath != "-" {
		baseDir = filepath.Dir(filepath.Clean(outputPath))
	}
	if filepath.Base(baseDir) == "dashboards" {
		return filepath.Join(filepath.Dir(baseDir), "semantic")
	}
	return filepath.Join(baseDir, "semantic")
}

var filenameSlugPattern = strings.NewReplacer(
	" ", "-",
	"_", "-",
	"/", "-",
	"\\", "-",
)

func slugifyFilename(name string) string {
	s := strings.ToLower(strings.TrimSpace(filenameSlugPattern.Replace(name)))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "metabase-dashboard"
	}
	return out
}
