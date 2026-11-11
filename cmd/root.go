package cmd

import (
	"context"
	"io/fs"

	"github.com/bruin-data/dac/pkg/telemetry"
	"github.com/urfave/cli/v3"
)

// frontendFS is set by Run to pass the embedded frontend to the serve command.
var frontendFS fs.FS

// buildInfoKey is the root command Metadata key holding the BuildInfo, so
// subcommands can read embedder settings via buildInfoFrom.
const buildInfoKey = "dac.buildInfo"

type BuildInfo struct {
	Version string
	Commit  string

	// BruinPath is the bruin binary dac runs queries through. Empty means
	// "bruin" looked up on PATH. An embedding binary such as the bruin CLI
	// sets this to its own executable so queries never reach another bruin.
	BruinPath string

	// Embedded marks dac as running inside another binary. It leaves out the
	// upgrade/update and version commands and the --version flag, which the
	// host binary covers, and takes the root command name from args[0] (for
	// example "bruin dac") so help output reads correctly.
	Embedded bool
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
	commands := []*cli.Command{
		withTelemetry(initCmd()),
		withTelemetry(serveCmd()),
		withTelemetry(buildCmd()),
		withTelemetry(validateCmd()),
		withTelemetry(checkCmd()),
		withTelemetry(queryCmd()),
		withTelemetry(lsCmd()),
		withTelemetry(connectionsCmd()),
		withTelemetry(skillsCmd()),
		withTelemetry(importCmd()),
		withTelemetry(exportCmd()),
	}
	if !build.Embedded {
		commands = append(commands,
			withTelemetry(upgradeCmd(build)),
			versionCmd(build),
		)
	}

	return &cli.Command{
		Name:        "dac",
		Usage:       "Dashboard-as-Code: define, validate, and serve dashboards from YAML and TSX",
		Version:     build.Version,
		HideVersion: build.Embedded,
		Metadata:    map[string]any{buildInfoKey: build},
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
		Commands: commands,
	}
}

func Run(ctx context.Context, args []string, frontend fs.FS, build BuildInfo) error {
	frontendFS = frontend
	app := NewApp(build)
	if build.Embedded && len(args) > 0 && args[0] != "" {
		app.Name = args[0]
	}
	return app.Run(ctx, args)
}

// buildInfoFrom returns the BuildInfo the root command was created with.
func buildInfoFrom(cmd *cli.Command) BuildInfo {
	build, _ := cmd.Root().Metadata[buildInfoKey].(BuildInfo)
	return build
}
