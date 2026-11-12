package template

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRender_IncludeWithinProject(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "snippets", "region.txt"), "region={{ filters.region }}")
	// A nested include resolves relative to the including file.
	writeFile(t, filepath.Join(root, "snippets", "outer.txt"), "[{% include 'region.txt' %}]")

	for tmpl, want := range map[string]string{
		"{% include 'snippets/region.txt' %}":             "region=EU",
		"{% include 'snippets/outer.txt' %}":              "[region=EU]",
		"{% include 'snippets/../snippets/region.txt' %}": "region=EU",
	} {
		got, err := Render(tmpl, map[string]any{"region": "EU"}, root)
		assertNoErr(t, err)
		assertEqual(t, got, want)
	}
}

func TestRender_IncludeConfinedToProject(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	secret := filepath.Join(parent, "secret.txt")
	writeFile(t, secret, "secret")
	writeFile(t, filepath.Join(root, "ok.txt"), "ok")
	if err := os.Symlink(secret, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(parent, filepath.Join(root, "linkdir")); err != nil {
		t.Fatal(err)
	}

	for name, tmpl := range map[string]string{
		"absolute path":       "{% include '" + secret + "' %}",
		"dot-dot escape":      "{% include '../secret.txt' %}",
		"nested escape":       "{% include 'sub/../../secret.txt' %}",
		"symlinked file":      "{% include 'link.txt' %}",
		"symlinked directory": "{% include 'linkdir/secret.txt' %}",
		"import":              "{% import '../secret.txt' as s %}",
		"extends":             "{% extends '../secret.txt' %}",
		"filter value path":   "{% include filters.path %}",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Render(tmpl, map[string]any{"path": secret}, root); err == nil {
				t.Fatalf("read outside the project: %q", got)
			}
		})
	}

	// Without a project root, or with a symlinked one, no file can be read,
	// but inline templates still render.
	link := filepath.Join(parent, "rootlink")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	for _, projectRoot := range []string{"", link} {
		if got, err := Render("{% include 'ok.txt' %}", nil, projectRoot); err == nil {
			t.Errorf("root %q: included %q", projectRoot, got)
		}
		got, err := Render("{{ 'inline' }}", nil, projectRoot)
		assertNoErr(t, err)
		assertEqual(t, got, "inline")
	}
}
