package cmd

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/server"
	"github.com/bruin-data/dac/pkg/telemetry"
	analytics "github.com/rudderlabs/analytics-go/v4"
	"github.com/urfave/cli/v3"
)

type databaseValidationResult struct {
	label      string
	connection string
	err        error
}

func validateCmd() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "Validate dashboard definitions and semantic model references",
		Flags: []cli.Flag{
			dirFlag,
			&cli.BoolFlag{
				Name:  "with-database",
				Usage: "Dry-run dashboard queries against configured database connections",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := dashboardDirFromCommand(cmd)
			if err != nil {
				return err
			}
			printSkillUpdateNotices(dir)
			withDatabase := cmd.Bool("with-database")

			var backend query.Backend
			var loadOptions []dashboard.TSXOption
			tsxLoadDryRuns := 0

			if withDatabase {
				configFile, _, err := resolveConfig(cmd, dir)
				if err != nil {
					return fmt.Errorf("config error: %w", err)
				}
				backend = newBackend(cmd, configFile)
				loadOptions = append(loadOptions, dashboard.WithQueryFunc(func(connection, sql string) (map[string]interface{}, error) {
					if err := dryRunQuery(ctx, backend, connection, sql); err != nil {
						return nil, err
					}
					tsxLoadDryRuns++
					return emptyTSXQueryResult(), nil
				}))
			}

			dashboards, err := loadDashboards(dir, loadOptions...)
			if err != nil {
				return err
			}
			if dashboards == nil {
				telemetry.SendEvent("dashboards_loaded", analytics.Properties{
					"count":         0,
					"valid_count":   0,
					"invalid_count": 0,
					"with_database": withDatabase,
				})
				fmt.Println("No dashboard files found in", dir)
				return nil
			}

			validCount := 0
			invalidCount := 0
			databaseDryRuns := tsxLoadDryRuns
			for _, d := range dashboards {
				if err := dashboard.Validate(d); err != nil {
					fmt.Println(err)
					invalidCount++
				} else {
					if withDatabase {
						results, err := validateDashboardWithDatabase(ctx, backend, d)
						if err != nil {
							fmt.Printf("  %s: %s\n", d.Name, err)
							invalidCount++
							continue
						}

						databaseDryRuns += len(results)
						failed := false
						fmt.Printf("\n%s\n", d.Name)
						fmt.Println("  ✓ structure")
						for _, r := range results {
							if r.err != nil {
								failed = true
								fmt.Printf("  ✗ %-30s  %s\n", r.label, r.err)
								continue
							}
							fmt.Printf("  ✓ %-30s  dry-run %s\n", r.label, r.connection)
						}
						if len(results) == 0 {
							fmt.Println("  ✓ no data queries")
						}
						if failed {
							invalidCount++
							continue
						}
					} else {
						fmt.Printf("  %s: OK\n", d.Name)
					}
					validCount++
				}
			}

			telemetry.SendEvent("dashboards_loaded", analytics.Properties{
				"count":         len(dashboards),
				"valid_count":   validCount,
				"invalid_count": invalidCount,
				"with_database": withDatabase,
				"dry_run_count": databaseDryRuns,
			})

			if invalidCount > 0 {
				return fmt.Errorf("validation failed")
			}

			fmt.Printf("\n%d dashboard(s) validated successfully.\n", len(dashboards))
			if withDatabase {
				if tsxLoadDryRuns > 0 {
					fmt.Printf("%d TSX load-time query dry-run(s) passed.\n", tsxLoadDryRuns)
				}
				fmt.Printf("%d database dry-run query(s) passed.\n", databaseDryRuns)
			}
			return nil
		},
	}
}

func validateDashboardWithDatabase(ctx context.Context, backend query.Backend, d *dashboard.Dashboard) ([]databaseValidationResult, error) {
	jobs, err := server.ResolveWidgetJobs(d, d.DefaultFilters())
	if err != nil {
		return nil, fmt.Errorf("query resolution failed: %w", err)
	}

	labels := widgetLabelsByID(d)
	results := make([]databaseValidationResult, 0, len(jobs))
	for _, job := range jobs {
		result := databaseValidationResult{
			label:      labelForWidgetJob(job, labels),
			connection: job.Connection,
		}
		if err := dryRunQuery(ctx, backend, job.Connection, job.SQL); err != nil {
			result.err = err
		}
		results = append(results, result)
	}
	return results, nil
}

func widgetLabelsByID(d *dashboard.Dashboard) map[string]string {
	labels := make(map[string]string)
	for rowIdx, row := range d.Rows {
		for widgetIdx, widget := range row.Widgets {
			labels[server.WidgetID(rowIdx, widgetIdx)] = widget.Name
		}
	}
	return labels
}

func labelForWidgetJob(job server.WidgetJob, labels map[string]string) string {
	if label := labels[job.ID]; label != "" {
		return label
	}
	return job.ID
}

func dryRunQuery(ctx context.Context, backend query.Backend, connection string, sql string) error {
	if err := validateDryRunSQL(sql); err != nil {
		return err
	}
	if dryRunner, ok := backend.(query.DryRunner); ok {
		_, err := dryRunner.DryRun(ctx, connection, sql)
		return err
	}
	_, err := backend.Execute(ctx, connection, explainSQL(sql))
	return err
}

func validateDryRunSQL(sql string) error {
	tokens, hasMultipleStatements := sqlTokens(sql)
	if len(tokens) == 0 {
		return fmt.Errorf("query is empty")
	}
	if hasMultipleStatements {
		return fmt.Errorf("database validation only supports a single read-only statement")
	}

	switch tokens[0] {
	case "select", "with", "values", "show", "describe", "explain":
	default:
		return fmt.Errorf("database validation only dry-runs read-only SELECT/WITH/VALUES/SHOW/DESCRIBE/EXPLAIN statements")
	}

	mutating := map[string]bool{
		"alter": true, "attach": true, "call": true, "copy": true,
		"create": true, "delete": true, "detach": true, "drop": true,
		"grant": true, "insert": true, "merge": true, "revoke": true,
		"set": true, "truncate": true, "update": true, "vacuum": true,
	}
	for _, token := range tokens {
		if mutating[token] {
			return fmt.Errorf("database validation refuses potentially mutating SQL containing %q", token)
		}
	}
	return nil
}

func explainSQL(sql string) string {
	trimmed := strings.TrimSpace(sql)
	if strings.HasPrefix(strings.ToLower(trimmed), "explain") {
		return trimmed
	}
	return "EXPLAIN " + trimmed
}

func sqlTokens(sql string) ([]string, bool) {
	var tokens []string
	multipleStatements := false
	for i := 0; i < len(sql); {
		r, size := utf8.DecodeRuneInString(sql[i:])

		if unicode.IsSpace(r) {
			i += size
			continue
		}
		if strings.HasPrefix(sql[i:], "--") {
			next := strings.IndexByte(sql[i:], '\n')
			if next == -1 {
				break
			}
			i += next + 1
			continue
		}
		if strings.HasPrefix(sql[i:], "/*") {
			next := strings.Index(sql[i+2:], "*/")
			if next == -1 {
				break
			}
			i += next + 4
			continue
		}
		if sql[i] == '\'' || sql[i] == '"' || sql[i] == '`' {
			i = skipQuotedSQL(sql, i, sql[i])
			continue
		}
		if sql[i] == ';' {
			if hasNonCommentSQL(sql[i+1:]) {
				multipleStatements = true
			}
			i++
			continue
		}
		if isSQLIdentStart(r) {
			start := i
			i += size
			for i < len(sql) {
				nextRune, nextSize := utf8.DecodeRuneInString(sql[i:])
				if !isSQLIdentPart(nextRune) {
					break
				}
				i += nextSize
			}
			tokens = append(tokens, strings.ToLower(sql[start:i]))
			continue
		}

		i += size
	}
	return tokens, multipleStatements
}

func hasNonCommentSQL(sql string) bool {
	tokens, _ := sqlTokens(sql)
	return len(tokens) > 0
}

func skipQuotedSQL(sql string, start int, quote byte) int {
	i := start + 1
	for i < len(sql) {
		if sql[i] == quote {
			if quote == '\'' && i+1 < len(sql) && sql[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1
		}
		if sql[i] == '\\' && i+1 < len(sql) {
			i += 2
			continue
		}
		i++
	}
	return len(sql)
}

func isSQLIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isSQLIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func emptyTSXQueryResult() map[string]interface{} {
	return map[string]interface{}{
		"columns": []interface{}{},
		"rows":    []interface{}{},
	}
}
