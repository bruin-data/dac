package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsList_PrintsBundledSkills(t *testing.T) {
	output := captureStdout(t, func() {
		if err := runSkillsList(); err != nil {
			t.Fatalf("skills list failed: %v", err)
		}
	})

	if !strings.Contains(output, "create-dashboard") {
		t.Fatalf("expected bundled skill in output, got %q", output)
	}
	if !strings.Contains(output, ".claude/skills/create-dashboard/SKILL.md") {
		t.Fatalf("expected Claude install target in output, got %q", output)
	}
	if !strings.Contains(output, ".codex/skills/create-dashboard") {
		t.Fatalf("expected Codex symlink target in output, got %q", output)
	}
}

func TestSkillsInstall_InstallsDefaultSkill(t *testing.T) {
	dir := t.TempDir()

	output := captureStdout(t, func() {
		if err := runSkillsInstall(dir, nil, false); err != nil {
			t.Fatalf("skills install failed: %v", err)
		}
	})

	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected skill file to be installed: %v", err)
	}
	if !strings.Contains(string(data), "name: create-dashboard") {
		t.Fatalf("installed skill content missing metadata: %s", data)
	}
	codexPath := filepath.Join(dir, ".codex", "skills", "create-dashboard")
	info, err := os.Lstat(codexPath)
	if err != nil {
		t.Fatalf("expected Codex skill symlink to be installed: %v", err)
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
	codexData, err := os.ReadFile(filepath.Join(codexPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("expected Codex symlinked skill to be readable: %v", err)
	}
	if string(codexData) != string(data) {
		t.Fatal("expected Codex symlink to point at the Claude skill content")
	}
	if !strings.Contains(output, "Restart your agent session") {
		t.Fatalf("expected restart guidance, got %q", output)
	}
}

func TestSkillsInstall_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := runSkillsInstall(dir, []string{"create-dashboard"}, false)
	if err == nil {
		t.Fatal("expected overwrite conflict")
	}
	if !strings.Contains(err.Error(), "would be overwritten") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestSkillsInstall_RefusesCodexOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".codex", "skills", "create-dashboard")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	err := runSkillsInstall(dir, []string{"create-dashboard"}, false)
	if err == nil {
		t.Fatal("expected overwrite conflict")
	}
	if !strings.Contains(err.Error(), ".codex/skills/create-dashboard") {
		t.Fatalf("expected Codex overwrite error, got %v", err)
	}
}

func TestSkillsInstall_ForceOverwritesSkill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := runSkillsInstall(dir, []string{"create-dashboard"}, true); err != nil {
		t.Fatalf("skills install --force failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if string(data) == "custom\n" {
		t.Fatalf("expected skill to be overwritten, got %s", data)
	}
	if info, err := os.Lstat(filepath.Join(dir, ".codex", "skills", "create-dashboard")); err != nil {
		t.Fatalf("expected Codex skill symlink: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected Codex skill path to be a symlink, got mode %s", info.Mode())
	}
}

func TestSkillsInstall_RejectsUnknownSkill(t *testing.T) {
	err := runSkillsInstall(t.TempDir(), []string{"missing"}, false)
	if err == nil {
		t.Fatal("expected unknown skill error")
	}
	if !strings.Contains(err.Error(), "unknown skill") {
		t.Fatalf("expected unknown skill error, got %v", err)
	}
}

func TestSkillsCommand_RegisteredWithApp(t *testing.T) {
	app := NewApp(BuildInfo{Version: "test"})
	var found bool
	for _, command := range app.Commands {
		if command.Name == "skills" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected skills command to be registered")
	}
}

func TestParseSkillVersion(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"bundled skill", createDashboardSkill, 2},
		{"explicit version", "---\nname: x\nversion: 7\n---\nbody", 7},
		{"missing version", "---\nname: x\n---\nbody", 0},
		{"no frontmatter", "# just a heading\n", 0},
		{"non-integer version", "---\nversion: 1.2\n---\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseSkillVersion(tc.content); got != tc.want {
				t.Fatalf("parseSkillVersion(%q) = %d, want %d", tc.name, got, tc.want)
			}
		})
	}
}

func TestSkillUpdateNotices_FlagsOlderInstall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	// An install with no version field reports version 0, older than bundled.
	if err := os.WriteFile(path, []byte("---\nname: create-dashboard\n---\nold body"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	notices := skillUpdateNotices(dir)
	if len(notices) != 1 {
		t.Fatalf("expected 1 notice, got %d: %v", len(notices), notices)
	}
	if !strings.Contains(notices[0], "create-dashboard") || !strings.Contains(notices[0], "dac skills update") {
		t.Fatalf("unexpected notice text: %q", notices[0])
	}
}

func TestSkillUpdateNotices_SilentWhenCurrentOrAbsent(t *testing.T) {
	// No .claude directory at all: no project root, no notices.
	if notices := skillUpdateNotices(t.TempDir()); len(notices) != 0 {
		t.Fatalf("expected no notices without an install, got %v", notices)
	}

	// Freshly installed skill is current with the bundled version.
	dir := t.TempDir()
	if err := runSkillsInstall(dir, nil, false); err != nil {
		t.Fatalf("skills install failed: %v", err)
	}
	if notices := skillUpdateNotices(dir); len(notices) != 0 {
		t.Fatalf("expected no notices for a current install, got %v", notices)
	}
}

func TestSkillUpdateNotices_DiscoversRootFromSubdir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("---\nname: create-dashboard\n---\nold"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	sub := filepath.Join(dir, "dashboards")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	if notices := skillUpdateNotices(sub); len(notices) != 1 {
		t.Fatalf("expected notice discovered from subdir, got %v", notices)
	}
}

func TestSkillsUpdate_OverwritesThroughCLI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("custom\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	app := NewApp(BuildInfo{Version: "test"})
	if err := app.Run(context.Background(), []string{"dac", "skills", "update", "--dir", dir}); err != nil {
		t.Fatalf("cli skills update failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if string(data) != createDashboardSkill {
		t.Fatalf("expected skill updated to bundled content, got %q", data)
	}
}

func TestSkillsCommand_RunsThroughCLI(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(BuildInfo{Version: "test"})

	if err := app.Run(context.Background(), []string{"dac", "skills", "install", "--dir", dir}); err != nil {
		t.Fatalf("cli skills install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "create-dashboard", "SKILL.md")); err != nil {
		t.Fatalf("expected installed skill: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(dir, ".codex", "skills", "create-dashboard")); err != nil {
		t.Fatalf("expected installed Codex symlink: %v", err)
	} else if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected Codex skill path to be a symlink, got mode %s", info.Mode())
	}
}
