package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
)

// standaloneHelpGolden holds `dac --help` and `<command> --help` for every
// visible command and subcommand of standalone dac. Regenerate it with
// DAC_UPDATE_GOLDEN=1 only when a help change is intended.
var standaloneHelpGolden = filepath.Join("testdata", "standalone_help.golden")

// helpPaths lists the argument paths of every visible command under app, root
// first, so each one can be run with --help.
func helpPaths(app *cli.Command) [][]string {
	paths := [][]string{nil}
	var walk func(prefix []string, cmds []*cli.Command)
	walk = func(prefix []string, cmds []*cli.Command) {
		for _, c := range cmds {
			if c.Hidden {
				continue
			}
			path := append(append([]string(nil), prefix...), c.Name)
			paths = append(paths, path)
			walk(path, c.Commands)
		}
	}
	walk(nil, app.Commands)
	return paths
}

// allHelp runs --help for every visible command and joins the output, each
// page under a "== <args>" header.
func allHelp(t *testing.T, argv0 string, build BuildInfo) string {
	t.Helper()
	var b strings.Builder
	for _, path := range helpPaths(NewApp(build)) {
		args := append(append([]string{argv0}, path...), "--help")
		out := captureStdout(t, func() {
			if err := Run(context.Background(), args, nil, build); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
		})
		b.WriteString("== " + strings.Join(append([]string{"dac"}, path...), " ") + " --help\n")
		b.WriteString(out)
	}
	return b.String()
}

func TestRun_StandaloneHelpUnchanged(t *testing.T) {
	build := BuildInfo{Version: "v1.2.3"}
	got := allHelp(t, "dac", build)

	if os.Getenv("DAC_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(standaloneHelpGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(standaloneHelpGolden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(standaloneHelpGolden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("standalone help changed; diff against %s:\n%s", standaloneHelpGolden, got)
	}

	// os.Args[0] is usually a path; standalone help always says "dac".
	for _, argv0 := range []string{"/usr/local/bin/dac", "./bin/dac"} {
		if other := allHelp(t, argv0, build); other != got {
			t.Fatalf("standalone help differs for argv0 %q:\n%s", argv0, other)
		}
	}
}
