package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bruin-data/dac/pkg/slides"
	"github.com/urfave/cli/v3"
)

func exportCmd() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export dashboards to external formats",
		Commands: []*cli.Command{
			exportSlidesCmd(),
		},
	}
}

func exportSlidesCmd() *cli.Command {
	return &cli.Command{
		Name:  "slides",
		Usage: "Export a dashboard to Google Slides",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "dashboard",
				Aliases:  []string{"n"},
				Usage:    "Dashboard name (required)",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "credentials",
				Usage: "Path to Google OAuth credentials.json (default: gcloud ADC)",
			},
			dirFlag,
			&cli.StringFlag{
				Name:  "filters",
				Usage: "JSON string with filter overrides",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir, err := dashboardDirFromCommand(cmd)
			if err != nil {
				return err
			}
			configFile := resolveConfigOptional(cmd, dir)

			var filters map[string]any
			if raw := cmd.String("filters"); raw != "" {
				if err := json.Unmarshal([]byte(raw), &filters); err != nil {
					return fmt.Errorf("invalid --filters JSON: %w", err)
				}
			}

			url, err := slides.Export(ctx, slides.Config{
				DashboardDir: dir,
				Dashboard:    cmd.String("dashboard"),
				Credentials:  cmd.String("credentials"),
				Filters:      filters,
				ConfigFile:   configFile,
				Environment:  cmd.Root().String("environment"),
				BruinPath:    buildInfoFrom(cmd).BruinPath,
			})
			if err != nil {
				return err
			}

			fmt.Printf("\nPresentation created: %s\n", url)
			return nil
		},
	}
}
