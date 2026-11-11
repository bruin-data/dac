package cmd

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/urfave/cli/v3"
)

const embedTestDashboard = `name: Embedded
connection: warehouse
rows:
  - widgets:
      - name: Count
        type: table
        sql: SELECT 1 AS n
`

// writeFakeBruin writes an executable `bruin` stand-in into dir that appends
// its arguments to logPath and answers like `bruin query --output json`.
func writeFakeBruin(t *testing.T, dir, logPath string) string {
	t.Helper()
	path := filepath.Join(dir, "bruin")
	script := `#!/bin/sh
printf '%s\n' "$@" >> "` + logPath + `"
printf '{"columns":[{"name":"n","type":"INTEGER"}],"rows":[[1]]}'
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bruin: %v", err)
	}
	return path
}

// embedFixture is a dac project plus two fake bruin binaries: "host" (the
// embedding binary, passed as BruinPath) and "decoy" (first on PATH).
type embedFixture struct {
	projectDir string
	configFile string
	hostBruin  string
	hostLog    string
	decoyLog   string
}

func newEmbedFixture(t *testing.T) embedFixture {
	t.Helper()
	root := t.TempDir()
	f := embedFixture{
		projectDir: filepath.Join(root, "project"),
		hostLog:    filepath.Join(root, "host.log"),
		decoyLog:   filepath.Join(root, "decoy.log"),
	}
	for _, dir := range []string{f.projectDir, filepath.Join(root, "host"), filepath.Join(root, "decoy")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f.configFile = filepath.Join(f.projectDir, ".bruin.yml")
	config := "default_environment: default\nenvironments:\n  default:\n    connections:\n      duckdb:\n        - name: warehouse\n          path: test.db\n"
	if err := os.WriteFile(f.configFile, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.projectDir, "embedded.yml"), []byte(embedTestDashboard), 0o644); err != nil {
		t.Fatal(err)
	}
	f.hostBruin = writeFakeBruin(t, filepath.Join(root, "host"), f.hostLog)
	writeFakeBruin(t, filepath.Join(root, "decoy"), f.decoyLog)
	t.Setenv("PATH", filepath.Join(root, "decoy")+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertHostOnly checks queries reached BruinPath and never the bruin on PATH.
func (f embedFixture) assertHostOnly(t *testing.T, wantSQL string) {
	t.Helper()
	host := readLog(t, f.hostLog)
	if !strings.Contains(host, "-q\n"+wantSQL+"\n") {
		t.Fatalf("expected host bruin to run %q, got log:\n%s", wantSQL, host)
	}
	if decoy := readLog(t, f.decoyLog); decoy != "" {
		t.Fatalf("bruin on PATH must not be called when BruinPath is set, got log:\n%s", decoy)
	}
}

func embeddedBuild(f embedFixture) BuildInfo {
	return BuildInfo{Version: "v1.2.3", BruinPath: f.hostBruin, Embedded: true}
}

func TestRun_BruinPathRunsQueriesThroughHostBinary(t *testing.T) {
	f := newEmbedFixture(t)

	args := []string{"bruin dac", "-c", f.configFile, "-e", "default", "query", "--connection", "warehouse", "--dir", f.projectDir, "-o", "json", "SELECT 42"}
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, embeddedBuild(f)); err != nil {
			t.Fatalf("query failed: %v", err)
		}
	})

	f.assertHostOnly(t, "SELECT 42")
	// Global flags are forwarded to bruin the same way as in standalone dac.
	host := readLog(t, f.hostLog)
	if !strings.Contains(host, "--config-file\n"+f.configFile+"\n") || !strings.Contains(host, "-e\ndefault\n") {
		t.Fatalf("expected -c/-e to reach bruin, got log:\n%s", host)
	}
}

func TestRun_BruinPathUsedByCheck(t *testing.T) {
	f := newEmbedFixture(t)

	captureStdout(t, func() {
		if err := Run(context.Background(), []string{"bruin dac", "check", "--dir", f.projectDir}, nil, embeddedBuild(f)); err != nil {
			t.Fatalf("check failed: %v", err)
		}
	})

	f.assertHostOnly(t, "SELECT 1 AS n")
}

func TestRun_BruinPathUsedByBuild(t *testing.T) {
	f := newEmbedFixture(t)
	frontend := fstest.MapFS{"index.html": {Data: []byte("<html><head></head><body></body></html>")}}
	out := filepath.Join(t.TempDir(), "out")

	args := []string{"bruin dac", "build", "--dir", f.projectDir, "--dashboard", "Embedded", "--output", out}
	captureStdout(t, func() {
		if err := Run(context.Background(), args, frontend, embeddedBuild(f)); err != nil {
			t.Fatalf("build failed: %v", err)
		}
	})

	f.assertHostOnly(t, "SELECT 1 AS n")
}

func TestRun_BruinPathUsedBySlidesExport(t *testing.T) {
	f := newEmbedFixture(t)
	// Make Google auth fail fast and offline; queries run before auth.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))

	args := []string{"bruin dac", "export", "slides", "--dir", f.projectDir, "--dashboard", "Embedded"}
	captureStdout(t, func() {
		_ = Run(context.Background(), args, nil, embeddedBuild(f))
	})

	f.assertHostOnly(t, "SELECT 1 AS n")
}

func TestRun_StandaloneUsesBruinOnPath(t *testing.T) {
	f := newEmbedFixture(t)

	args := []string{"dac", "query", "--connection", "warehouse", "--dir", f.projectDir, "-o", "json", "SELECT 7"}
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, BuildInfo{Version: "v1.2.3"}); err != nil {
			t.Fatalf("query failed: %v", err)
		}
	})

	if decoy := readLog(t, f.decoyLog); !strings.Contains(decoy, "-q\nSELECT 7\n") {
		t.Fatalf("standalone dac should run bruin from PATH, got log:\n%s", decoy)
	}
	if host := readLog(t, f.hostLog); host != "" {
		t.Fatalf("unexpected host bruin call:\n%s", host)
	}
}

func commandNames(t *testing.T, build BuildInfo) []string {
	t.Helper()
	var names []string
	for _, c := range NewApp(build).Commands {
		names = append(names, c.Names()...)
	}
	return names
}

func TestNewApp_StandaloneCommandsUnchanged(t *testing.T) {
	want := []string{
		"init", "serve", "build", "validate", "check", "query", "ls",
		"connections", "skills", "import", "export", "upgrade", "update", "version",
	}
	if got := commandNames(t, BuildInfo{Version: "v1.2.3"}); !slices.Equal(got, want) {
		t.Fatalf("standalone commands changed:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestNewApp_EmbeddedLeavesOutUpgradeAndVersion(t *testing.T) {
	got := commandNames(t, BuildInfo{Version: "v1.2.3", Embedded: true})
	for _, name := range []string{"upgrade", "update", "version"} {
		if slices.Contains(got, name) {
			t.Fatalf("embedded app must not register %q, got %v", name, got)
		}
	}
	if !slices.Contains(got, "serve") || !slices.Contains(got, "skills") {
		t.Fatalf("embedded app is missing regular commands: %v", got)
	}
}

func TestRun_EmbeddedRejectsUpgradeAndVersion(t *testing.T) {
	// Unknown commands go through urfave/cli's exit handler, as in standalone dac.
	oldExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = oldExiter })
	cli.OsExiter = func(int) {}

	for _, args := range [][]string{
		{"bruin dac", "upgrade"},
		{"bruin dac", "update"},
		{"bruin dac", "version"},
		{"bruin dac", "--version"},
	} {
		var err error
		out := captureStdout(t, func() {
			err = Run(context.Background(), args, nil, BuildInfo{Version: "v1.2.3", Embedded: true})
		})
		if err == nil {
			t.Fatalf("%v: expected an error, got output %q", args, out)
		}
		if strings.Contains(out, "v1.2.3") {
			t.Fatalf("%v: embedded dac printed its own version: %q", args, out)
		}
	}
}

// standaloneRootHelp is `dac --help` as printed before embedding support was
// added. Standalone help must stay byte-for-byte the same.
const standaloneRootHelp = `NAME:
   dac - Dashboard-as-Code: define, validate, and serve dashboards from YAML and TSX

USAGE:
   dac [global options] [command [command options]]

VERSION:
   v1.2.3

COMMANDS:
   init             Create a new DAC project
   serve            Start development server with live reload
   build            Build a static dashboard with baked-in query results
   validate         Validate dashboard definitions and semantic model references
   check            Validate dashboards and execute all queries to verify they work
   query            Run a SQL query against a connection
   ls               List discovered dashboards
   connections      Test database connections from .bruin.yml
   skills           List, install, and update DAC agent skills
   import           Import dashboards from external tools
   export           Export dashboards to external formats
   upgrade, update  Upgrade the dac CLI in place to the latest release
   version          Show DAC version information
   help, h          Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config string, -c string       Path to .bruin.yml config file (default: auto-discover)
   --environment string, -e string  Target environment name
   --debug                          Enable debug logging
   --help, -h                       show help
   --version, -v                    print the version
`

func TestRun_StandaloneHelpUnchanged(t *testing.T) {
	// os.Args[0] is usually a path; standalone help always says "dac".
	for _, argv0 := range []string{"dac", "/usr/local/bin/dac", "./bin/dac"} {
		out := captureStdout(t, func() {
			if err := Run(context.Background(), []string{argv0, "--help"}, nil, BuildInfo{Version: "v1.2.3"}); err != nil {
				t.Fatalf("help failed: %v", err)
			}
		})
		if out != standaloneRootHelp {
			t.Fatalf("standalone help changed for argv0 %q:\n%s", argv0, out)
		}
	}
}

func TestRun_EmbeddedHelpUsesHostName(t *testing.T) {
	build := BuildInfo{Version: "v1.2.3", Embedded: true}

	root := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"bruin dac", "--help"}, nil, build); err != nil {
			t.Fatalf("help failed: %v", err)
		}
	})
	for _, want := range []string{
		"bruin dac - Dashboard-as-Code",
		"bruin dac [global options] [command [command options]]",
		"serve ", "check ", "skills ",
		"--config string, -c string", "--environment string, -e string", "--debug",
	} {
		if !strings.Contains(root, want) {
			t.Fatalf("embedded help missing %q:\n%s", want, root)
		}
	}
	for _, unwanted := range []string{"bruin dac dac", "upgrade", "version", "VERSION:", "v1.2.3"} {
		if strings.Contains(root, unwanted) {
			t.Fatalf("embedded help must not contain %q:\n%s", unwanted, root)
		}
	}

	serve := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"bruin dac", "serve", "--help"}, nil, build); err != nil {
			t.Fatalf("serve help failed: %v", err)
		}
	})
	if !strings.Contains(serve, "bruin dac serve - Start development server") || !strings.Contains(serve, "bruin dac serve [options]") {
		t.Fatalf("embedded serve help should read `bruin dac serve`:\n%s", serve)
	}
}

func TestSkillRefreshHint(t *testing.T) {
	standalone := skillRefreshHint(NewApp(BuildInfo{}))("/work/project")
	if standalone != "Run `dac upgrade` to update the CLI and refresh your project's copy." {
		t.Fatalf("standalone hint changed, got %q", standalone)
	}

	app := NewApp(BuildInfo{Embedded: true})
	app.Name = "bruin dac"
	hint := skillRefreshHint(app)
	if got := hint("/work/project"); got != "Run `bruin dac skills update --dir /work/project` to refresh your project's copy." {
		t.Fatalf("unexpected embedded hint %q", got)
	}
	if got := hint("/work/my project"); !strings.Contains(got, "--dir '/work/my project'`") {
		t.Fatalf("embedded hint should quote paths with spaces, got %q", got)
	}
}

// Following the embedded notice from a dashboard subdirectory must refresh the
// stale copy in the project root, not install a second copy in the subdirectory.
func TestEmbeddedSkillNoticeFromSubdirUpdatesProjectRoot(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: create-dashboard\n---\nold body"), 0o644); err != nil {
		t.Fatal(err)
	}
	dashboards := filepath.Join(root, "dashboards")
	if err := os.MkdirAll(dashboards, 0o755); err != nil {
		t.Fatal(err)
	}

	app := NewApp(BuildInfo{Embedded: true})
	app.Name = "bruin dac"
	notices := skillUpdateNotices(dashboards, skillRefreshHint(app))
	if len(notices) != 1 {
		t.Fatalf("expected 1 notice, got %v", notices)
	}
	_, after, _ := strings.Cut(notices[0], "Run `")
	command, _, _ := strings.Cut(after, "`")
	fields := strings.Fields(command)
	if len(fields) < 3 || strings.Join(fields[:2], " ") != "bruin dac" {
		t.Fatalf("unexpected hint command %q", command)
	}

	t.Chdir(dashboards)
	args := append([]string{"bruin dac"}, fields[2:]...)
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, BuildInfo{Embedded: true}); err != nil {
			t.Fatalf("%v failed: %v", args, err)
		}
	})

	data, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != createDashboardSkill {
		t.Fatalf("project root skill was not refreshed")
	}
	if _, err := os.Stat(filepath.Join(dashboards, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("hint installed a second skill copy in the dashboard dir (err=%v)", err)
	}
}

func TestRun_EmbeddedInitNextStepsUseHostName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")
	build := BuildInfo{Embedded: true, BruinPath: "/opt/bruin/bin/bruin"}

	out := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"bruin dac", "init", dir}, nil, build); err != nil {
			t.Fatalf("init failed: %v", err)
		}
	})
	if !strings.Contains(out, "  bruin dac validate --dir .\n") || !strings.Contains(out, "  bruin dac serve --dir . --open\n") {
		t.Fatalf("embedded next steps should use `bruin dac`, got:\n%s", out)
	}
	if strings.Contains(out, "on your PATH") {
		t.Fatalf("embedded dac runs queries through BruinPath, PATH hint is misleading:\n%s", out)
	}
}

func TestRun_StandaloneInitNextStepsUnchanged(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project")

	out := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"/usr/local/bin/dac", "init", dir}, nil, BuildInfo{Version: "v1.2.3"}); err != nil {
			t.Fatalf("init failed: %v", err)
		}
	})
	want := "  dac validate --dir .\n  dac serve --dir . --open\n\nIf you want to execute live queries, make sure the Bruin CLI is installed and on your PATH.\n"
	if !strings.HasSuffix(out, want) {
		t.Fatalf("standalone next steps changed, got:\n%s", out)
	}
}
