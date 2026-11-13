package cmd

import (
	"context"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

func connectionsCmd() *cli.Command {
	return &cli.Command{
		Name:  "connections",
		Usage: "Test database connections from .bruin.yml",
		Flags: []cli.Flag{dirFlag},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := dashboardDirFromCommand(cmd)
			if err != nil {
				return err
			}

			configFile, cfg, err := resolveConfig(cmd, dir)
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			envName := cmd.Root().String("environment")
			env, err := cfg.GetEnvironment(envName)
			if err != nil {
				return err
			}

			backend := newBackend(cmd, configFile, cfg)

			type connResult struct {
				name     string
				connType string
				status   string
				isError  bool
			}

			var results []connResult
			var mu sync.Mutex
			var wg sync.WaitGroup

			for connType, conns := range env.Connections {
				for _, conn := range conns {
					wg.Add(1)
					go func(name, ct string) {
						defer wg.Done()
						_, err := backend.Execute(ctx, name, "SELECT 1")
						r := connResult{name: name, connType: ct}
						if err != nil {
							r.status = "✗ " + summarizeError(err)
							r.isError = true
						} else {
							r.status = "✓ connected"
						}
						mu.Lock()
						results = append(results, r)
						mu.Unlock()
					}(conn.Name, connType)
				}
			}

			wg.Wait()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tSTATUS")

			hasErrors := false
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\n", r.name, r.connType, r.status)
				if r.isError {
					hasErrors = true
				}
			}

			w.Flush()

			if hasErrors {
				return fmt.Errorf("some connections failed")
			}
			return nil
		},
	}
}

// summarizeError extracts a short message from a bruin query error.
func summarizeError(err error) string {
	s := err.Error()
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}
