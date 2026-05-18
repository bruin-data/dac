package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"
)

// installScriptURLFmt is the canonical installer used by `dac upgrade`. The
// ref is pinned to the running binary's version (a known-good release commit)
// so a broken `main` cannot break every user's upgrade path. Dev/test builds
// fall back to `main`.
const installScriptURLFmt = "https://raw.githubusercontent.com/bruin-data/dac/%s/install.sh"

// installScriptURL returns the install.sh URL pinned to the version of the
// running binary, or to `main` for dev/test builds where no release commit
// exists.
func installScriptURL(version string) string {
	ref := version
	if ref == "" || ref == "dev" || strings.HasPrefix(ref, "test-") {
		ref = "main"
	}
	return fmt.Sprintf(installScriptURLFmt, ref)
}

func upgradeCmd(build BuildInfo) *cli.Command {
	return &cli.Command{
		Name:    "upgrade",
		Aliases: []string{"update"},
		Usage:   "Upgrade the dac CLI in place to the latest release",
		Description: "Re-runs the official install script to replace the current binary " +
			"with the latest release on the same channel. Pass --channel edge to track " +
			"the edge stream, or a tag like v1.2.3 as an argument to pin to a specific version.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "channel",
				Usage: "Release channel to install from (stable or edge)",
			},
			&cli.StringFlag{
				Name:  "bindir",
				Usage: "Directory to install into (default: directory of the current binary)",
			},
		},
		ArgsUsage: "[tag]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runUpgrade(ctx, cmd, build)
		},
	}
}

func runUpgrade(ctx context.Context, cmd *cli.Command, build BuildInfo) error {
	channel := strings.TrimSpace(cmd.String("channel"))
	if channel == "" {
		channel = inferChannel(build.Version)
	}
	if channel != "stable" && channel != "edge" {
		return fmt.Errorf("--channel must be 'stable' or 'edge', got %q", channel)
	}

	bindir := strings.TrimSpace(cmd.String("bindir"))
	if bindir == "" {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("could not locate current binary: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(exe)
		if err == nil {
			exe = resolved
		}
		bindir = filepath.Dir(exe)
	}

	tag := strings.TrimSpace(cmd.Args().First())

	fmt.Fprintf(os.Stderr, "Upgrading dac (%s channel) into %s...\n", channel, bindir)

	args := []string{"-fsSL", installScriptURL(build.Version)}
	curl := exec.CommandContext(ctx, "curl", args...)
	curlOut, err := curl.StdoutPipe()
	if err != nil {
		return err
	}
	curl.Stderr = os.Stderr

	bashArgs := []string{"-s", "--", "-b", bindir, "--channel", channel}
	if tag != "" {
		bashArgs = append(bashArgs, tag)
	}
	bash := exec.CommandContext(ctx, "bash", bashArgs...)
	bash.Stdin = curlOut
	bash.Stdout = os.Stderr // installer logs to stdout; route to stderr so CLI output stays clean
	bash.Stderr = os.Stderr
	// The official installer would otherwise reinstall the bruin CLI on every
	// dac upgrade — skip it; users who want bruin can run that installer
	// themselves.
	bash.Env = append(os.Environ(), "DAC_SKIP_BRUIN_INSTALL=1")

	if err := bash.Start(); err != nil {
		return fmt.Errorf("could not start install script: %w", err)
	}
	if err := curl.Start(); err != nil {
		_ = bash.Process.Kill()
		return fmt.Errorf("could not start curl: %w", err)
	}
	if err := curl.Wait(); err != nil {
		_ = bash.Process.Kill()
		return fmt.Errorf("curl failed: %w", err)
	}
	if err := bash.Wait(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	return nil
}

// inferChannel picks a default channel from the running binary's version
// string. Edge tags look like v0.0.0-edge.NNN.SHA1234.
func inferChannel(version string) string {
	if strings.Contains(version, "-edge.") {
		return "edge"
	}
	return "stable"
}
