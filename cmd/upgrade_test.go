package cmd

import (
	"strings"
	"testing"
)

func TestInferChannel(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"v1.2.3", "stable"},
		{"1.2.3", "stable"},
		{"", "stable"},
		{"dev", "stable"},
		{"v0.0.0-edge.42.abc123", "edge"},
		{"v0.0.0-edge.1.deadbe", "edge"},
	}
	for _, c := range cases {
		if got := inferChannel(c.version); got != c.want {
			t.Errorf("inferChannel(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}

func TestInstallScriptURL(t *testing.T) {
	cases := []struct {
		version  string
		wantRef  string
		wantPath string
	}{
		{"v1.2.3", "v1.2.3", "/bruin-data/dac/v1.2.3/install.sh"},
		{"v0.0.0-edge.42.abc", "v0.0.0-edge.42.abc", "/bruin-data/dac/v0.0.0-edge.42.abc/install.sh"},
		{"dev", "main", "/bruin-data/dac/main/install.sh"},
		{"test-debug", "main", "/bruin-data/dac/main/install.sh"},
		{"", "main", "/bruin-data/dac/main/install.sh"},
	}
	for _, c := range cases {
		got := installScriptURL(c.version)
		if !strings.Contains(got, c.wantPath) {
			t.Errorf("installScriptURL(%q) = %q, want path containing %q", c.version, got, c.wantPath)
		}
		if !strings.HasPrefix(got, "https://raw.githubusercontent.com/") {
			t.Errorf("installScriptURL(%q) = %q, want raw.githubusercontent.com prefix", c.version, got)
		}
	}
}
