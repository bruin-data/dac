package cmd

import (
	"context"
	"io/fs"
	"os"

	"github.com/bruin-data/dac/pkg/telemetry"
	"github.com/urfave/cli/v3"
)

// frontendFS is set by Run to pass the embedded frontend to the serve command.
var frontendFS fs.FS

// buildInfoKey is the root command Metadata key holding the BuildInfo, so
// subcommands can read it via buildInfoFrom.
const buildInfoKey = "dac.buildInfo"

type BuildInfo struct {
	Version string
	Commit  string

	// BruinPath is the bruin binary every query runs through. Empty means
	// "bruin" looked up on PATH. Set from DAC_BRUIN_PATH.
	BruinPath string

	// BruinHost marks dac as launched by the bruin CLI as `bruin dac`. It
	// hides the upgrade/update and version commands and the --version flag,
	// and names the program `bruin dac` in help and in commands dac tells the
	// user to run. Set by DAC_HOST=bruin; any other value leaves it false.
	BruinHost bool
}

// WithHostEnv returns b with BruinPath and BruinHost read from DAC_BRUIN_PATH
// and DAC_HOST. main calls it once at startup; nothing else in dac reads them.
func (b BuildInfo) WithHostEnv() BuildInfo {
	b.BruinPath = os.Getenv("DAC_BRUIN_PATH")
	b.BruinHost = os.Getenv("DAC_HOST") == "bruin"
	return b
}

// programName is the command users type to run dac.
func (b BuildInfo) programName() string {
	if b.BruinHost {
		return "bruin dac"
	}
	return "dac"
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
	// bruin manages the dac install and has its own version command.
	if !build.BruinHost {
		commands = append(commands,
			withTelemetry(upgradeCmd(build)),
			versionCmd(build),
		)
	}

	return &cli.Command{
		Name:        build.programName(),
		Usage:       "Dashboard-as-Code: define, validate, and serve dashboards from YAML and TSX",
		Version:     build.Version,
		HideVersion: build.BruinHost,
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
	return NewApp(build).Run(ctx, args)
}

// buildInfoFrom returns the BuildInfo the root command was created with.
func buildInfoFrom(cmd *cli.Command) BuildInfo {
	build, _ := cmd.Root().Metadata[buildInfoKey].(BuildInfo)
	return build
}
