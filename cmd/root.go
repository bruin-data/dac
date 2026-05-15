package cmd

import (
	"context"
	"io/fs"

	"github.com/bruin-data/dac/pkg/telemetry"
	"github.com/urfave/cli/v3"
)

// frontendFS is set by Run to pass the embedded frontend to the serve command.
var frontendFS fs.FS

type BuildInfo struct {
	Version string
	Commit  string
}

// withTelemetry attaches telemetry Before/After hooks to a command. Hooks are
// attached per-subcommand (matching Bruin CLI) so they only fire when a real
// command runs, not on `dac --help` or `dac --version`.
func withTelemetry(c *cli.Command) *cli.Command {
	c.Before = telemetry.BeforeCommand
	c.After = telemetry.AfterCommand
	return c
}

func NewApp(build BuildInfo) *cli.Command {
	return &cli.Command{
		Name:    "dac",
		Usage:   "Dashboard-as-Code: define, validate, and serve dashboards from YAML and TSX",
		Version: build.Version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to .bruin.yml config file (default: auto-discover)",
			},
			&cli.StringFlag{
				Name:    "environment",
				Aliases: []string{"e"},
				Usage:   "Target environment name",
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
			},
		},
		Commands: []*cli.Command{
			withTelemetry(initCmd()),
			withTelemetry(serveCmd()),
			withTelemetry(buildCmd()),
			withTelemetry(validateCmd()),
			withTelemetry(checkCmd()),
			withTelemetry(queryCmd()),
			withTelemetry(lsCmd()),
			withTelemetry(connectionsCmd()),
			withTelemetry(skillsCmd()),
			withTelemetry(exportCmd()),
			withTelemetry(upgradeCmd(build)),
			versionCmd(build),
		},
	}
}

func Run(ctx context.Context, args []string, frontend fs.FS, build BuildInfo) error {
	frontendFS = frontend
	return NewApp(build).Run(ctx, args)
}
