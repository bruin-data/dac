package query

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBruinCLIBackendDryRun(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args.txt")
	bruinPath := filepath.Join(dir, "bruin")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + argsPath + `"
printf '{"connectionName":"warehouse","connectionType":"duckdb","query":"SELECT 1","valid":true}'
`
	if err := os.WriteFile(bruinPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bruin: %v", err)
	}

	backend := &BruinCLIBackend{
		BruinPath:   bruinPath,
		ConfigFile:  "bruin.yml",
		Environment: "dev",
	}
	result, err := backend.DryRun(context.Background(), "warehouse", "SELECT 1")
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if !result.Valid || result.ConnectionName != "warehouse" || result.ConnectionType != "duckdb" {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}

	data, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	want := []string{
		"query",
		"--output",
		"json",
		"--dry-run",
		"-c",
		"warehouse",
		"--config-file",
		"bruin.yml",
		"-e",
		"dev",
		"-q",
		"SELECT 1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected args:\ngot:  %q\nwant: %q", got, want)
	}
}

func TestBruinCLIBackendDryRunReturnsBruinError(t *testing.T) {
	dir := t.TempDir()
	bruinPath := filepath.Join(dir, "bruin")
	script := `#!/bin/sh
printf '{"error":"table not found"}'
exit 1
`
	if err := os.WriteFile(bruinPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake bruin: %v", err)
	}

	backend := &BruinCLIBackend{BruinPath: bruinPath}
	_, err := backend.DryRun(context.Background(), "warehouse", "SELECT * FROM missing")
	if err == nil {
		t.Fatal("expected dry-run error")
	}
	if !strings.Contains(err.Error(), "table not found") {
		t.Fatalf("expected bruin error in message, got %v", err)
	}
}

func TestConnectionTypeReadsConfigUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".bruin.yml")
	definition := "default_environment: prod\nenvironments:\n  prod:\n    connections:\n      google_cloud_platform:\n        - name: warehouse\n"
	if err := os.WriteFile(path, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	// A long-running server must see config edits rather than reuse a stale snapshot.
	server := NewCachedBackend(&BruinCLIBackend{ConfigFile: path}, 0)
	if got, err := ConnectionType("warehouse", server); err != nil || got != "google_cloud_platform" {
		t.Fatalf("initial config: %q, %v", got, err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(definition, "google_cloud_platform", "postgres")), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ConnectionType("warehouse", server); err != nil || got != "postgres" {
		t.Fatalf("updated config: %q, %v", got, err)
	}
}
