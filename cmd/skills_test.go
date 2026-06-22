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
