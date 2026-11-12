package template

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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
		"filter value escape": "{% include filters.rel %}",
	} {
		t.Run(name, func(t *testing.T) {
			if got, err := Render(tmpl, map[string]any{"path": secret, "rel": "../secret.txt"}, root); err == nil {
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

// A template that includes itself must fail, not exhaust memory.
func TestRender_IncludeDepthLimited(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "self.txt"), "x{% include 'self.txt' %}")
	writeFile(t, filepath.Join(root, "dashboard.yml"), "value: \"{% include filters.path %}\"")
	for _, tmpl := range []string{"{% include 'self.txt' %}", "{% include filters.path %}"} {
		if got, err := Render(tmpl, map[string]any{"path": "dashboard.yml"}, root); err == nil {
			t.Errorf("%q: recursive include rendered %d bytes", tmpl, len(got))
		}
	}
	// "ignore missing" skips the depth error; fan-out must still stop.
	writeFile(t, filepath.Join(root, "fan.txt"), "{% include 'fan.txt' ignore missing %}{% include 'fan.txt' ignore missing %}")
	done := make(chan error, 1)
	go func() {
		_, err := Render("{% include 'fan.txt' %}", nil, root)
		done <- err
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("fan-out include did not stop")
	}

	// Nesting below the limit still works.
	for i := 0; i < maxIncludeDepth-1; i++ {
		writeFile(t, filepath.Join(root, "chain", fmt.Sprintf("%d.txt", i)), fmt.Sprintf("{%% include '%d.txt' %%}", i+1))
	}
	writeFile(t, filepath.Join(root, "chain", fmt.Sprintf("%d.txt", maxIncludeDepth-1)), "end")
	got, err := Render("{% include 'chain/0.txt' %}", nil, root)
	assertNoErr(t, err)
	assertEqual(t, got, "end")
}
