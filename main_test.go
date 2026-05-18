package main

import "testing"

func TestShouldSkipNudge(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"bare invocation", []string{"dac"}, true},
		{"version subcommand", []string{"dac", "version"}, true},
		{"upgrade subcommand", []string{"dac", "upgrade"}, true},
		{"update alias", []string{"dac", "update"}, true},
		{"help subcommand", []string{"dac", "help"}, true},
		{"h alias", []string{"dac", "h"}, true},
		{"--version flag", []string{"dac", "--version"}, true},
		{"--help flag", []string{"dac", "--help"}, true},
		{"-v short flag", []string{"dac", "-v"}, true},
		{"serve subcommand", []string{"dac", "serve"}, false},
		{"validate subcommand", []string{"dac", "validate"}, false},
		{"global flag then serve", []string{"dac", "--debug", "serve"}, false},
		{"help with target", []string{"dac", "help", "serve"}, true},
		{"upgrade with tag", []string{"dac", "upgrade", "v1.2.3"}, true},
	}
	for _, c := range cases {
		if got := shouldSkipNudge(c.args); got != c.want {
			t.Errorf("%s: shouldSkipNudge(%v) = %v, want %v", c.name, c.args, got, c.want)
		}
	}
}
