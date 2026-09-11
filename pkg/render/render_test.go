package render

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestWriteStaticOutput_ClearsStaleAssets(t *testing.T) {
	out := t.TempDir()

	// Simulate an older build already present in the output directory.
	if err := os.MkdirAll(filepath.Join(out, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(out, "assets", "index-OLDHASH.js")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	frontend := fstest.MapFS{
		"index.html":              {Data: []byte("<head></head>")},
		"assets/index-NEWHASH.js": {Data: []byte("new")},
		"assets/embed-ABC.js":     {Data: []byte("vega")},
	}

	if err := writeStaticOutput(out, frontend, "<head>injected</head>"); err != nil {
		t.Fatalf("writeStaticOutput: %v", err)
	}

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale asset should be removed on rebuild, got err=%v", err)
	}
	for _, f := range []string{"assets/index-NEWHASH.js", "assets/embed-ABC.js"} {
		if _, err := os.Stat(filepath.Join(out, f)); err != nil {
			t.Errorf("expected %s to be copied: %v", f, err)
		}
	}
	index, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(index) != "<head>injected</head>" {
		t.Errorf("index.html should be the modified content, got %q", index)
	}
}
