package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/urfave/cli/v3"
)

const hostTestDashboard = `name: Hosted
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

// hostFixture is a dac project plus two fake bruin binaries: "host", which
// DAC_BRUIN_PATH points at, and "decoy", which is first on PATH.
type hostFixture struct {
	projectDir string
	configFile string
	hostBruin  string
	hostLog    string
	decoyLog   string
}

// newHostFixture builds the fixture and puts the decoy first on PATH. It
// clears DAC_BRUIN_PATH and DAC_HOST so each test sets only what it needs.
func newHostFixture(t *testing.T) hostFixture {
	t.Helper()
	root := t.TempDir()
	f := hostFixture{
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
	if err := os.WriteFile(filepath.Join(f.projectDir, "hosted.yml"), []byte(hostTestDashboard), 0o644); err != nil {
		t.Fatal(err)
	}
	f.hostBruin = writeFakeBruin(t, filepath.Join(root, "host"), f.hostLog)
	writeFakeBruin(t, filepath.Join(root, "decoy"), f.decoyLog)
	t.Setenv("PATH", filepath.Join(root, "decoy")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DAC_BRUIN_PATH", "")
	t.Setenv("DAC_HOST", "")
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

// assertHostOnly checks queries reached DAC_BRUIN_PATH and never the bruin on PATH.
func (f hostFixture) assertHostOnly(t *testing.T, wantSQL string) {
	t.Helper()
	host := readLog(t, f.hostLog)
	if !strings.Contains(host, "-q\n"+wantSQL+"\n") {
		t.Fatalf("expected DAC_BRUIN_PATH bruin to run %q, got log:\n%s", wantSQL, host)
	}
	if decoy := readLog(t, f.decoyLog); decoy != "" {
		t.Fatalf("bruin on PATH must not be called when DAC_BRUIN_PATH is set, got log:\n%s", decoy)
	}
}

// envBuild is what main passes to Run: the build info plus the host env.
func envBuild() BuildInfo {
	return BuildInfo{Version: "v1.2.3"}.WithHostEnv()
}

func TestWithHostEnv(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"bruin", true},
		{"", false},
		{"Bruin", false},
		{"bruin ", false},
		{"dac", false},
	}
	for _, c := range cases {
		t.Setenv("DAC_HOST", c.host)
		t.Setenv("DAC_BRUIN_PATH", "/opt/bruin/bin/bruin")
		got := BuildInfo{Version: "v1"}.WithHostEnv()
		if got.BruinHost != c.want {
			t.Errorf("DAC_HOST=%q: BruinHost = %v, want %v", c.host, got.BruinHost, c.want)
		}
		if got.BruinPath != "/opt/bruin/bin/bruin" || got.Version != "v1" {
			t.Errorf("DAC_HOST=%q: unexpected build info %+v", c.host, got)
		}
	}
}

func TestDACBruinPath_QueryPathsUseIt(t *testing.T) {
	cases := []struct {
		name    string
		args    func(f hostFixture) []string
		wantSQL string
		wantErr bool
	}{
		{
			name: "query",
			args: func(f hostFixture) []string {
				return []string{"query", "--connection", "warehouse", "--dir", f.projectDir, "-o", "json", "SELECT 42"}
			},
			wantSQL: "SELECT 42",
		},
		{
			name:    "check",
			args:    func(f hostFixture) []string { return []string{"check", "--dir", f.projectDir} },
			wantSQL: "SELECT 1 AS n",
		},
		{
			name:    "validate --with-database",
			args:    func(f hostFixture) []string { return []string{"validate", "--with-database", "--dir", f.projectDir} },
			wantSQL: "SELECT 1 AS n",
		},
		{
			name:    "connections",
			args:    func(f hostFixture) []string { return []string{"connections", "--dir", f.projectDir} },
			wantSQL: "SELECT 1",
		},
		{
			name: "build",
			args: func(f hostFixture) []string {
				return []string{"build", "--dir", f.projectDir, "--dashboard", "Hosted", "--output", filepath.Join(f.projectDir, "out")}
			},
			wantSQL: "SELECT 1 AS n",
		},
		{
			// Queries run before Google auth, which then fails offline.
			name: "export slides",
			args: func(f hostFixture) []string {
				return []string{"export", "slides", "--dir", f.projectDir, "--dashboard", "Hosted"}
			},
			wantSQL: "SELECT 1 AS n",
			wantErr: true,
		},
	}
	frontend := fstest.MapFS{"index.html": {Data: []byte("<html><head></head><body></body></html>")}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newHostFixture(t)
			t.Setenv("DAC_BRUIN_PATH", f.hostBruin)
			t.Setenv("HOME", t.TempDir())
			t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))

			var err error
			captureStdout(t, func() {
				err = Run(context.Background(), append([]string{"dac"}, c.args(f)...), frontend, envBuild())
			})
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			f.assertHostOnly(t, c.wantSQL)
		})
	}
}

func TestDACBruinPath_GlobalFlagsForwarded(t *testing.T) {
	f := newHostFixture(t)
	t.Setenv("DAC_BRUIN_PATH", f.hostBruin)

	args := []string{"dac", "-c", f.configFile, "-e", "default", "query", "--connection", "warehouse", "--dir", f.projectDir, "-o", "json", "SELECT 42"}
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, envBuild()); err != nil {
			t.Fatalf("query failed: %v", err)
		}
	})

	host := readLog(t, f.hostLog)
	if !strings.Contains(host, "--config-file\n"+f.configFile+"\n") || !strings.Contains(host, "-e\ndefault\n") {
		t.Fatalf("expected -c/-e to reach bruin, got log:\n%s", host)
	}
}

func TestDACBruinPath_UnsetUsesBruinOnPath(t *testing.T) {
	f := newHostFixture(t)

	args := []string{"dac", "query", "--connection", "warehouse", "--dir", f.projectDir, "-o", "json", "SELECT 7"}
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, envBuild()); err != nil {
			t.Fatalf("query failed: %v", err)
		}
	})

	if decoy := readLog(t, f.decoyLog); !strings.Contains(decoy, "-q\nSELECT 7\n") {
		t.Fatalf("without DAC_BRUIN_PATH dac should run bruin from PATH, got log:\n%s", decoy)
	}
	if host := readLog(t, f.hostLog); host != "" {
		t.Fatalf("unexpected host bruin call:\n%s", host)
	}
}

// TestServeHelperProcess runs `dac serve` with the build info main would pass,
// for TestDACBruinPath_ServeUsesIt. It does nothing in a normal test run.
func TestServeHelperProcess(t *testing.T) {
	if os.Getenv("DAC_TEST_SERVE_HELPER") != "1" {
		t.Skip("helper process for TestDACBruinPath_ServeUsesIt")
	}
	args := []string{"dac", "serve", "--dir", os.Getenv("DAC_TEST_SERVE_DIR"), "--port", os.Getenv("DAC_TEST_SERVE_PORT")}
	if err := Run(context.Background(), args, nil, envBuild()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// serve blocks until the process exits, so it runs in a child process that
// the test kills when done.
func TestDACBruinPath_ServeUsesIt(t *testing.T) {
	f := newHostFixture(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	child := exec.Command(os.Args[0], "-test.run=^TestServeHelperProcess$")
	child.Env = append(os.Environ(),
		"DAC_TEST_SERVE_HELPER=1",
		"DAC_TEST_SERVE_DIR="+f.projectDir,
		fmt.Sprintf("DAC_TEST_SERVE_PORT=%d", port),
		"DAC_BRUIN_PATH="+f.hostBruin,
	)
	stderr, err := child.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})

	// The server picks the next free port if ours was taken; read the one it
	// reports.
	addrCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			if _, addr, ok := strings.Cut(scanner.Text(), "listening on "); ok {
				addrCh <- addr
				break
			}
		}
		_, _ = io.Copy(io.Discard, stderr)
	}()
	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(30 * time.Second):
		t.Fatal("dac serve did not start")
	}

	resp, err := http.Post(addr+"/api/v1/query", "application/json", strings.NewReader(`{"connection":"warehouse","sql":"SELECT 99"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("query status %d: %s", resp.StatusCode, body)
	}
	f.assertHostOnly(t, "SELECT 99")
}

func commandNames(build BuildInfo) []string {
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
	if got := commandNames(BuildInfo{Version: "v1.2.3"}); !slices.Equal(got, want) {
		t.Fatalf("standalone commands changed:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestDACHost_HidesUpgradeAndVersion(t *testing.T) {
	t.Setenv("DAC_HOST", "bruin")
	got := commandNames(envBuild())
	for _, name := range []string{"upgrade", "update", "version"} {
		if slices.Contains(got, name) {
			t.Fatalf("DAC_HOST=bruin must not register %q, got %v", name, got)
		}
	}
	if !slices.Contains(got, "serve") || !slices.Contains(got, "skills") {
		t.Fatalf("DAC_HOST=bruin is missing regular commands: %v", got)
	}

	// Unknown commands go through urfave/cli's exit handler, as in standalone dac.
	oldExiter := cli.OsExiter
	t.Cleanup(func() { cli.OsExiter = oldExiter })
	cli.OsExiter = func(int) {}

	for _, arg := range []string{"upgrade", "update", "version", "--version", "-v"} {
		var err error
		out := captureStdout(t, func() {
			err = Run(context.Background(), []string{"dac", arg}, nil, envBuild())
		})
		if err == nil {
			t.Fatalf("%s: expected an error, got output %q", arg, out)
		}
		if strings.Contains(out, "v1.2.3") {
			t.Fatalf("%s: dac printed its own version under bruin: %q", arg, out)
		}
	}
}

func TestDACHost_HelpSaysBruinDac(t *testing.T) {
	t.Setenv("DAC_HOST", "bruin")
	// os.Args[0] is the dac binary's path, never "bruin dac".
	argv0 := "/home/me/.bruin/dac/v1.2.3/dac"

	root := captureStdout(t, func() {
		if err := Run(context.Background(), []string{argv0, "--help"}, nil, envBuild()); err != nil {
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
			t.Fatalf("help missing %q:\n%s", want, root)
		}
	}
	for _, unwanted := range []string{"bruin dac dac", "upgrade", "version", "VERSION:", "v1.2.3", argv0} {
		if strings.Contains(root, unwanted) {
			t.Fatalf("help must not contain %q:\n%s", unwanted, root)
		}
	}

	// Every visible subcommand's help names itself `bruin dac <command>`.
	for _, path := range helpPaths(NewApp(envBuild()))[1:] {
		name := "bruin dac " + strings.Join(path, " ")
		out := captureStdout(t, func() {
			if err := Run(context.Background(), append(append([]string{argv0}, path...), "--help"), nil, envBuild()); err != nil {
				t.Fatalf("%s --help: %v", name, err)
			}
		})
		if !strings.Contains(out, "NAME:\n   "+name+" - ") || !strings.Contains(out, "USAGE:\n   "+name+" ") {
			t.Fatalf("%s --help should name %q:\n%s", name, name, out)
		}
		if strings.Contains(out, "bruin dac dac") {
			t.Fatalf("%s --help repeats dac:\n%s", name, out)
		}
	}
}

// Any DAC_HOST other than "bruin" is unset: help is byte-for-byte standalone.
func TestDACHost_OtherValuesKeepStandaloneHelp(t *testing.T) {
	want, err := os.ReadFile(standaloneHelpGolden)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"", "dac", "BRUIN", "bruin-cloud"} {
		t.Setenv("DAC_HOST", host)
		if got := allHelp(t, "dac", envBuild()); got != string(want) {
			t.Fatalf("DAC_HOST=%q changed standalone help:\n%s", host, got)
		}
	}
}

func TestDACHost_InitNextSteps(t *testing.T) {
	t.Setenv("DAC_HOST", "bruin")
	t.Setenv("DAC_BRUIN_PATH", "/opt/bruin/bin/bruin")
	dir := filepath.Join(t.TempDir(), "project")

	out := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"dac", "init", dir}, nil, envBuild()); err != nil {
			t.Fatalf("init failed: %v", err)
		}
	})
	if !strings.HasSuffix(out, "  bruin dac validate --dir .\n  bruin dac serve --dir . --open\n") {
		t.Fatalf("next steps should use `bruin dac` and drop the PATH hint, got:\n%s", out)
	}
}

func TestStandaloneInitNextStepsUnchanged(t *testing.T) {
	t.Setenv("DAC_HOST", "")
	t.Setenv("DAC_BRUIN_PATH", "")
	dir := filepath.Join(t.TempDir(), "project")

	out := captureStdout(t, func() {
		if err := Run(context.Background(), []string{"/usr/local/bin/dac", "init", dir}, nil, envBuild()); err != nil {
			t.Fatalf("init failed: %v", err)
		}
	})
	want := "  dac validate --dir .\n  dac serve --dir . --open\n\nIf you want to execute live queries, make sure the Bruin CLI is installed and on your PATH.\n"
	if !strings.HasSuffix(out, want) {
		t.Fatalf("standalone next steps changed, got:\n%s", out)
	}
}

func TestDACHost_UnknownSkillHint(t *testing.T) {
	err := runSkillsInstall(t.TempDir(), []string{"missing"}, false, BuildInfo{BruinHost: true})
	if err == nil || !strings.Contains(err.Error(), "run `bruin dac skills list`") {
		t.Fatalf("expected a `bruin dac skills list` hint, got %v", err)
	}
	err = runSkillsInstall(t.TempDir(), []string{"missing"}, false, BuildInfo{})
	if err == nil || !strings.Contains(err.Error(), "run `dac skills list`") {
		t.Fatalf("standalone hint changed, got %v", err)
	}
}

func writeStaleSkill(t *testing.T, root string) string {
	t.Helper()
	skillPath := filepath.Join(root, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: create-dashboard\n---\nold body"), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillPath
}

func TestSkillUpdateNotice_Hint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "my project")
	writeStaleSkill(t, root)

	standalone := skillUpdateNotices(root, BuildInfo{})
	if len(standalone) != 1 || !strings.HasSuffix(standalone[0], "\nRun `dac upgrade` to update the CLI and refresh your project's copy.") {
		t.Fatalf("standalone notice changed: %q", standalone)
	}

	hosted := skillUpdateNotices(root, BuildInfo{BruinHost: true})
	want := "\nRun `bruin dac skills update --dir '" + root + "'` to refresh your project's copy."
	if len(hosted) != 1 || !strings.HasSuffix(hosted[0], want) {
		t.Fatalf("expected notice ending %q, got %q", want, hosted)
	}
	if strings.Contains(hosted[0], "upgrade") {
		t.Fatalf("notice must not point at upgrade under bruin: %q", hosted[0])
	}
}

// Following the bruin notice from a dashboard subdirectory must refresh the
// stale copy in the project root, not install a second copy in the subdirectory.
func TestDACHost_SkillNoticeFromSubdirUpdatesProjectRoot(t *testing.T) {
	t.Setenv("DAC_HOST", "bruin")
	root := t.TempDir()
	skillPath := writeStaleSkill(t, root)
	dashboards := filepath.Join(root, "dashboards")
	if err := os.MkdirAll(dashboards, 0o755); err != nil {
		t.Fatal(err)
	}

	notices := skillUpdateNotices(dashboards, envBuild())
	if len(notices) != 1 {
		t.Fatalf("expected 1 notice, got %v", notices)
	}
	_, after, _ := strings.Cut(notices[0], "Run `")
	command, _, _ := strings.Cut(after, "`")
	fields := strings.Fields(command)
	if len(fields) < 3 || strings.Join(fields[:2], " ") != "bruin dac" {
		t.Fatalf("unexpected hint command %q", command)
	}

	// bruin runs dac with the arguments after `bruin dac`.
	t.Chdir(dashboards)
	args := append([]string{"dac"}, fields[2:]...)
	captureStdout(t, func() {
		if err := Run(context.Background(), args, nil, envBuild()); err != nil {
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
