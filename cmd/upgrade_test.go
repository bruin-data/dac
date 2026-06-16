package cmd

import (
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
