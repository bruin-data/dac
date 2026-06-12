package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/bruin-data/dac/pkg/config"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/server"
	"github.com/bruin-data/dac/pkg/telemetry"
	analytics "github.com/rudderlabs/analytics-go/v4"
	"github.com/urfave/cli/v3"
)

func queryCmd() *cli.Command {
	return &cli.Command{
		Name:  "query",
		Usage: "Run a SQL query against a connection",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "connection",
				Aliases: []string{"c"},
				Usage:   "Connection name from .bruin.yml",
			},
			&cli.StringFlag{
				Name:    "file",
				Aliases: []string{"f"},
				Usage:   "Path to a .sql file to execute",
			},
			dirFlag,
			&cli.StringFlag{
				Name:  "dashboard",
				Usage: "Dashboard name (to run a specific widget's query)",
			},
			&cli.StringFlag{
				Name:    "widget",
				Aliases: []string{"w"},
				Usage:   "Widget name within the dashboard",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output format: table, json, csv",
				Value:   "table",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.String("dir")

			configFile, cfg, err := resolveConfig(cmd, dir)
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			backend := newBackend(cmd, configFile)

			var sql, connection, source string

			dashboardName := cmd.String("dashboard")
			widgetName := cmd.String("widget")

			if dashboardName != "" && widgetName != "" {
				source = "dashboard"
				sql, connection, err = resolveWidgetQuery(dir, dashboardName, widgetName)
				if err != nil {
					return err
				}
			} else if cmd.String("file") != "" {
				source = "file"
				data, err := os.ReadFile(cmd.String("file"))
				if err != nil {
					return fmt.Errorf("reading SQL file: %w", err)
				}
				sql = string(data)
				connection = cmd.String("connection")
			} else if cmd.Args().Len() > 0 {
				source = "inline"
				sql = strings.Join(cmd.Args().Slice(), " ")
				connection = cmd.String("connection")
			} else {
				return fmt.Errorf("provide SQL as argument, --file, or --dashboard/--widget")
			}

			if connection == "" {
				return fmt.Errorf("no connection specified: use --connection or run against a dashboard widget")
			}

			result, execErr := backend.Execute(ctx, connection, sql)
			telemetry.SendEvent("query_executed", analytics.Properties{
				"source":          source,
				"output":          cmd.String("output"),
				"connection_type": connectionType(cfg, cmd.Root().String("environment"), connection),
				"success":         execErr == nil,
			})
			if execErr != nil {
				return fmt.Errorf("query failed: %w", execErr)
			}

			return printResult(result, cmd.String("output"))
		},
	}
}

// connectionType resolves a connection name to its type bucket (e.g. "duckdb",
// "bigquery") for telemetry. Returns empty string if not found — telemetry is
// best-effort and never blocks the user's query.
func connectionType(cfg *config.Config, environment, name string) string {
	if cfg == nil {
		return ""
	}
	env, err := cfg.GetEnvironment(environment)
	if err != nil || env == nil {
		return ""
	}
	for connType, conns := range env.Connections {
		for _, c := range conns {
			if c.Name == name {
				return connType
			}
		}
	}
	return ""
}

// resolveWidgetQuery finds a widget in a dashboard and returns its resolved, templated SQL.
func resolveWidgetQuery(dir, dashboardName, widgetName string) (string, string, error) {
	dashboards, err := dashboard.LoadDir(dir)
	if err != nil {
		return "", "", fmt.Errorf("loading dashboards: %w", err)
	}

	d := dashboard.FindByName(dashboards, dashboardName)
	if d == nil {
		return "", "", fmt.Errorf("dashboard %q not found", dashboardName)
	}
	if err := dashboard.Validate(d); err != nil {
		return "", "", fmt.Errorf("dashboard validation failed: %w", err)
	}

	var widgetID string
	for i, row := range d.Rows {
		for j, w := range row.Widgets {
			if w.Name != widgetName {
				continue
			}
			widgetID = server.WidgetID(i, j)
			break
		}
	}

	if widgetID == "" {
		return "", "", fmt.Errorf("widget %q not found in dashboard %q", widgetName, dashboardName)
	}

	jobs, err := server.ResolveWidgetJobs(d, d.DefaultFilters())
	if err != nil {
		return "", "", fmt.Errorf("resolving widget jobs: %w", err)
	}

	for _, job := range jobs {
		if job.ID == widgetID {
			return job.SQL, job.Connection, nil
		}
	}

	return "", "", fmt.Errorf("widget %q did not produce a query", widgetName)
}

func printResult(result *query.QueryResult, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	case "csv":
		w := csv.NewWriter(os.Stdout)
		header := make([]string, len(result.Columns))
		for i, col := range result.Columns {
			header[i] = col.Name
		}
		if err := w.Write(header); err != nil {
			return err
		}
		for _, row := range result.Rows {
			record := make([]string, len(row))
			for i, v := range row {
				record[i] = fmt.Sprintf("%v", v)
			}
			if err := w.Write(record); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()

	default: // table
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		header := make([]string, len(result.Columns))
		for i, col := range result.Columns {
			header[i] = col.Name
		}
		fmt.Fprintln(tw, strings.Join(header, "\t"))

		sep := make([]string, len(result.Columns))
		for i, col := range result.Columns {
			sep[i] = strings.Repeat("─", max(len(col.Name), 4))
		}
		fmt.Fprintln(tw, strings.Join(sep, "\t"))

		for _, row := range result.Rows {
			vals := make([]string, len(row))
			for i, v := range row {
				vals[i] = formatCell(v)
			}
			fmt.Fprintln(tw, strings.Join(vals, "\t"))
		}

		fmt.Fprintf(tw, "\n(%d rows)\n", len(result.Rows))
		return tw.Flush()
	}
}

func formatCell(v any) string {
	if v == nil {
		return "NULL"
	}
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%.2f", val)
	default:
		s := fmt.Sprintf("%v", val)
		if len(s) > 60 {
			return s[:57] + "..."
		}
		return s
	}
}
