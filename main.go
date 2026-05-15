package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bruin-data/dac/cmd"
	"github.com/bruin-data/dac/pkg/telemetry"
	"github.com/bruin-data/dac/pkg/update"
)

// updateWaitOnExit is how long we'll wait for the update check goroutine to
// finish before printing the nudge. The check itself caps HTTP at 3s, but at
// process exit we'd rather skip the nudge than make `dac ls` feel slow.
const updateWaitOnExit = 1 * time.Second

var (
	version      = "dev"
	commit       = ""
	telemetryKey = ""
)

func main() {
	if telemetryKey == "" {
		telemetryKey = os.Getenv("TELEMETRY_KEY")
	}
	telemetry.TelemetryKey = telemetryKey
	telemetry.OptOut = os.Getenv("TELEMETRY_OPTOUT") != "" || os.Getenv("DO_NOT_TRACK") != ""
	telemetry.AppVersion = version
	client := telemetry.Init()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Kick off the update check in the background. The receive is gated on a
	// short timeout below so a slow GitHub round-trip never blocks the user.
	var updateCh <-chan update.Result
	if checker := update.NewChecker(version); checker != nil {
		updateCh = checker.Start(ctx)
	}

	buildInfo := cmd.BuildInfo{
		Version: version,
		Commit:  commit,
	}

	args := os.Args
	err := cmd.Run(ctx, args, frontendDistFS(), buildInfo)
	if err == nil {
		printUpdateNudge(args, updateCh)
		return
	}

	// Close manually since defer doesn't run on os.Exit.
	client.Close()
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// printUpdateNudge surfaces the cached/fresh update check result if it
// arrived in time, but only for commands where the nudge is appropriate.
func printUpdateNudge(args []string, ch <-chan update.Result) {
	if ch == nil {
		return
	}
	if shouldSkipNudge(args) {
		return
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return
		}
		update.Nudge(os.Stderr, res)
	case <-time.After(updateWaitOnExit):
		// Check still in flight; skip rather than make the CLI feel slow.
		// The goroutine will keep running until the process exits, which
		// is fine — its only side effect is writing ~/.dac/update-check.json.
	}
}

// shouldSkipNudge suppresses the nudge for commands whose output should stay
// clean (version, help) or where the nudge would be redundant (upgrade,
// update).
func shouldSkipNudge(args []string) bool {
	for _, a := range args[1:] {
		if len(a) > 0 && a[0] == '-' {
			continue // skip flags
		}
		switch a {
		case "version", "upgrade", "update", "help", "h":
			return true
		}
		return false
	}
	return true // no subcommand → don't nudge
}
