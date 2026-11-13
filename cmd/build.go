package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bruin-data/dac/pkg/render"
	"github.com/urfave/cli/v3"
)

func buildCmd() *cli.Command {
	return &cli.Command{
		Name:  "build",
		Usage: "Build a static dashboard with baked-in query results",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "dashboard",
				Aliases:  []string{"n"},
				Usage:    "Dashboard name (required)",
				Required: true,
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output directory",
				Value:   "build",
			},
			dirFlag,
			&cli.StringFlag{
				Name:    "template",
				Aliases: []string{"t"},
				Usage:   "Template name or path to a theme YAML file",
				Value:   "bruin",
			},
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

			return render.Build(ctx, render.Config{
				DashboardDir: dir,
				Dashboard:    cmd.String("dashboard"),
				OutputDir:    cmd.String("output"),
				Filters:      filters,
				TemplateName: cmd.String("template"),
				ConfigFile:   configFile,
				Environment:  cmd.Root().String("environment"),
				BruinPath:    buildInfoFrom(cmd).BruinPath,
				Frontend:     frontendFS,
			})
		},
	}
}
