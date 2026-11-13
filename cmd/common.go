package cmd

import (
	"fmt"

	"github.com/bruin-data/dac/pkg/config"
	"github.com/bruin-data/dac/pkg/dashboard"
	"github.com/bruin-data/dac/pkg/query"
	"github.com/urfave/cli/v3"
)

// dirFlag is the shared --dir/-d flag used by most commands.
var dirFlag = &cli.StringFlag{
	Name:    "dir",
	Aliases: []string{"d"},
	Usage:   "Dashboard definitions directory",
	Value:   ".",
}

// resolveConfig discovers and loads the .bruin.yml config file.
func resolveConfig(cmd *cli.Command, dir string) (string, *config.Config, error) {
	configFile := cmd.Root().String("config")
	if configFile == "" {
		found, err := config.Discover(dir)
		if err != nil {
			return "", nil, err
		}
		configFile = found
	}

	cfg, err := config.Load(configFile)
	if err != nil {
		return configFile, nil, err
	}

	return configFile, cfg, nil
}

// resolveConfigOptional is like resolveConfig but treats missing config as non-fatal.
// Returns empty configFile and nil config if not found.
func resolveConfigOptional(cmd *cli.Command, dir string) string {
	configFile := cmd.Root().String("config")
	if configFile == "" {
		found, err := config.Discover(dir)
		if err != nil {
			return ""
		}
		configFile = found
	}
	return configFile
}

// newBackend creates a BruinCLIBackend from the resolved config.
func newBackend(cmd *cli.Command, configFile string, cfg *config.Config) *query.BruinCLIBackend {
	return &query.BruinCLIBackend{
		BruinPath:   buildInfoFrom(cmd).BruinPath,
		ConfigFile:  configFile,
		Config:      cfg,
		Environment: cmd.Root().String("environment"),
	}
}

// loadDashboards loads dashboards from a directory, returning a user-friendly error if empty.
func loadDashboards(dir string, opts ...dashboard.TSXOption) ([]*dashboard.Dashboard, error) {
	dashboards, err := dashboard.LoadDir(dir, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load dashboards: %w", err)
	}
	if len(dashboards) == 0 {
		return nil, nil
	}
	return dashboards, nil
}

func dashboardDirFromCommand(cmd *cli.Command) (string, error) {
	if cmd.Args().Len() == 0 {
		return cmd.String("dir"), nil
	}
	if cmd.Args().Len() > 1 {
		return "", fmt.Errorf("expected at most one dashboard directory argument")
	}
	if dir := cmd.String("dir"); dir != "." {
		return "", fmt.Errorf("pass the dashboard directory either as an argument or with --dir, not both")
	}
	return cmd.Args().First(), nil
}

func loadValidatedDashboards(dir string) ([]*dashboard.Dashboard, error) {
	dashboards, err := loadDashboards(dir)
	if err != nil || dashboards == nil {
		return dashboards, err
	}
	if err := dashboard.ValidateAll(dashboards); err != nil {
		return nil, err
	}
	return dashboards, nil
}
