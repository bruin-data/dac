package cmd

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/bruin-data/dac/pkg/server"
	"github.com/bruin-data/dac/pkg/telemetry"
	analytics "github.com/rudderlabs/analytics-go/v4"
	"github.com/urfave/cli/v3"
)

type checkJob struct {
	job server.WidgetJob
}

type checkResult struct {
	widgetID string
	rows     int
	cols     int
	elapsed  time.Duration
	err      error
}

func checkCmd() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Validate dashboards and execute all queries to verify they work",
		Flags: []cli.Flag{dirFlag},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := dashboardDirFromCommand(cmd)
			if err != nil {
				return err
			}

			configFile, _, err := resolveConfig(cmd, dir)
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			backend := newBackend(cmd, configFile)

			dashboards, err := loadDashboards(dir)
			if err != nil {
				return err
			}
			telemetry.SendEvent("dashboards_loaded", analytics.Properties{
				"count": len(dashboards),
			})
			if dashboards == nil {
				fmt.Println("No dashboard files found in", dir)
				return nil
			}

			totalErrors := 0
			totalWidgets := 0

			for _, d := range dashboards {
				fmt.Printf("\n%s\n", d.Name)

				if err := dashboard.Validate(d); err != nil {
					fmt.Printf("  ✗ YAML validation failed\n")
					if ve, ok := err.(*dashboard.ValidationError); ok {
						for _, e := range ve.Errors {
							fmt.Printf("    %s\n", e)
						}
					} else {
						fmt.Printf("    %s\n", err)
					}
					totalErrors++
					continue
				}

				defaults := d.DefaultFilters()

				// Collect jobs, tracking text widgets separately (no query needed).
				var passiveWidgets []string
				widgetNames := make(map[string]string)

				for rowIdx, row := range d.Rows {
					for widgetIdx, w := range row.Widgets {
						totalWidgets++
						widgetID := server.WidgetID(rowIdx, widgetIdx)
						widgetNames[widgetID] = w.Name

						if w.Type == dashboard.WidgetTypeText || w.Type == dashboard.WidgetTypeDivider || w.Type == dashboard.WidgetTypeImage {
							passiveWidgets = append(passiveWidgets, w.Name)
							continue
						}
					}
				}

				for _, name := range passiveWidgets {
					fmt.Printf("  ✓ %-30s  static\n", name)
				}

				jobs, err := server.ResolveWidgetJobs(d, defaults)
				if err != nil {
					fmt.Printf("  ✗ query resolution failed: %s\n", err)
					totalErrors++
					continue
				}
				connGroups := make(map[string][]checkJob)
				for _, job := range jobs {
					connGroups[job.Connection] = append(connGroups[job.Connection], checkJob{job: job})
				}

				results := executeCheckJobs(ctx, backend, connGroups)
				for _, r := range results {
					name := widgetNames[r.widgetID]
					if r.err != nil {
						fmt.Printf("  ✗ %-30s  %s\n", name, r.err)
						totalErrors++
					} else {
						fmt.Printf("  ✓ %-30s  %d rows, %d cols  (%dms)\n",
							name, r.rows, r.cols, r.elapsed.Milliseconds())
					}
				}
			}

			fmt.Println()
			if totalErrors > 0 {
				fmt.Printf("%d error(s) across %d dashboard(s), %d widget(s) checked\n",
					totalErrors, len(dashboards), totalWidgets)
				return fmt.Errorf("check failed with %d error(s)", totalErrors)
			}

			fmt.Printf("%d dashboard(s), %d widget(s) — all passing\n", len(dashboards), totalWidgets)
			return nil
		},
	}
}

// executeCheckJobs runs query jobs grouped by connection. Same connection runs
// sequentially (avoids file lock contention), different connections in parallel.
func executeCheckJobs(ctx context.Context, backend query.Backend, connGroups map[string][]checkJob) []checkResult {
	var allResults []checkResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, jobs := range connGroups {
		wg.Add(1)
		go func(jobs []checkJob) {
			defer wg.Done()
			for _, j := range jobs {
				start := time.Now()
				wr := server.ExecuteWidgetQuery(ctx, backend, j.job)
				elapsed := time.Since(start)

				result := widgetResultToCheckResult(j.job.ID, wr, elapsed)

				mu.Lock()
				allResults = append(allResults, result)
				mu.Unlock()
			}
		}(jobs)
	}

	wg.Wait()
	return allResults
}

func widgetResultToCheckResult(widgetID string, wr *server.WidgetQueryResult, elapsed time.Duration) checkResult {
	r := checkResult{
		widgetID: widgetID,
		elapsed:  elapsed,
	}
	if wr.Error != "" {
		r.err = fmt.Errorf("%s", wr.Error)
		return r
	}
	r.rows = len(wr.Rows)
	r.cols = len(wr.Columns)
	return r
}
