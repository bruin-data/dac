package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bruin-data/dac/pkg/dashboard"
)

func TestInitCommand_CreatesStarterProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "starter")

	output := captureStdout(t, func() {
		if err := runInit(dir, "starter", false, "dac", true); err != nil {
			t.Fatalf("init failed: %v", err)
		}
	})

	for _, path := range []string{
		".bruin.yml",
		"README.md",
		"data/dac-demo.duckdb",
		".claude/skills/create-dashboard/SKILL.md",
		"dashboards/sales.yml",
		"dashboards/semantic-sales.yml",
		"semantic/sales.yml",
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
	assertGitRepository(t, dir)
	config, err := os.ReadFile(filepath.Join(dir, ".bruin.yml"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(config), "read_only: true") {
		t.Fatalf("expected DuckDB connection to be read-only, got %s", config)
	}
	assertCodexSkillSymlink(t, dir)

	dashboards, err := dashboard.LoadDir(dir)
	if err != nil {
		t.Fatalf("loading generated dashboards: %v", err)
	}
	if len(dashboards) != 2 {
		t.Fatalf("expected 2 dashboards, got %d", len(dashboards))
	}
	if err := dashboard.ValidateAll(dashboards); err != nil {
		t.Fatalf("generated dashboards should validate: %v", err)
	}
	if !strings.Contains(output, "dac serve --dir . --open") {
		t.Fatalf("expected next steps in output, got %q", output)
	}
}

func TestInitCommand_SQLTemplateSkipsSemanticFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sql")

	if err := runInit(dir, "sql", false, "dac", true); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dashboards/sales.yml")); err != nil {
		t.Fatalf("expected SQL dashboard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "semantic")); !os.IsNotExist(err) {
		t.Fatalf("expected semantic directory to be absent, got err=%v", err)
	}
	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	if strings.Contains(string(readme), "Semantic Sales") {
		t.Fatalf("sql template README should not reference semantic dashboard: %s", readme)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")); err != nil {
		t.Fatalf("expected SQL template to install agent skill: %v", err)
	}
	assertCodexSkillSymlink(t, dir)
}

func TestInitCommand_TemplatesAliases(t *testing.T) {
	cases := map[string]string{
		"basic-yaml":    "sql",
		"semantic-yml":  "semantic",
		"semantic-yaml": "semantic",
		"semantic-tsx":  "tsx",
		"yaml":          "sql",
	}

	for input, want := range cases {
		if got := normalizeInitTemplate(input); got != want {
			t.Fatalf("normalizeInitTemplate(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestInitCommand_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".bruin.yml"), []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := runInit(dir, "starter", false, "dac", true)
	if err == nil {
		t.Fatal("expected overwrite conflict")
	}
	if !strings.Contains(err.Error(), "would be overwritten") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestInitCommand_ForceOverwritesScaffoldFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bruin.yml")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := runInit(dir, "starter", true, "dac", true); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), "existing") {
		t.Fatalf("expected config to be overwritten, got %q", data)
	}
}

func TestInitCommand_RefusesSkillOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "skills", "create-dashboard")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := runInit(dir, "starter", false, "dac", true)
	if err == nil {
		t.Fatal("expected overwrite conflict")
	}
	if !strings.Contains(err.Error(), ".codex/skills/create-dashboard") {
		t.Fatalf("expected Codex skill conflict, got %v", err)
	}
}

func TestInitCommand_ForceReplacesCodexSkillDirectoryWithSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "skills", "create-dashboard")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := runInit(dir, "starter", true, "dac", true); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}
	assertCodexSkillSymlink(t, dir)
}

func TestInitGitRepository_MissingGitReturnsFriendlyError(t *testing.T) {
	t.Setenv("PATH", "")

	err := initGitRepository(t.TempDir())
	if err == nil {
		t.Fatal("expected missing git error")
	}
	for _, want := range []string{
		"git is required",
		"Bruin uses the Git repository root",
		"git init",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestInitCommand_RegisteredWithApp(t *testing.T) {
	app := NewApp(BuildInfo{Version: "test"})
	var found bool
	for _, command := range app.Commands {
		if command.Name == "init" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected init command to be registered")
	}
}

func TestInitCommand_RunsThroughCLI(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cli")
	app := NewApp(BuildInfo{Version: "test"})

	if err := app.Run(context.Background(), []string{"dac", "init", "--template", "semantic", dir}); err != nil {
		t.Fatalf("cli init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dashboards/semantic-sales.yml")); err != nil {
		t.Fatalf("expected semantic dashboard: %v", err)
	}
	assertGitRepository(t, dir)
	assertCodexSkillSymlink(t, dir)
}

func assertGitRepository(t *testing.T, dir string) {
	t.Helper()

	headPath := filepath.Join(dir, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		t.Fatalf("expected generated project to be a git repository: %v", err)
	}
	if !strings.HasPrefix(string(data), "ref: refs/heads/") {
		t.Fatalf("unexpected git HEAD content %q", data)
	}
}

func assertCodexSkillSymlink(t *testing.T, dir string) {
	t.Helper()

	codexPath := filepath.Join(dir, ".codex", "skills", "create-dashboard")
	info, err := os.Lstat(codexPath)
	if err != nil {
		t.Fatalf("expected Codex skill symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected Codex skill path to be a symlink, got mode %s", info.Mode())
	}
	target, err := os.Readlink(codexPath)
	if err != nil {
		t.Fatalf("read Codex symlink: %v", err)
	}
	if target != filepath.Join("..", "..", ".claude", "skills", "create-dashboard") {
		t.Fatalf("unexpected Codex symlink target %q", target)
	}
	if _, err := os.Stat(filepath.Join(codexPath, "SKILL.md")); err != nil {
		t.Fatalf("expected Codex skill symlink to expose SKILL.md: %v", err)
	}
}
