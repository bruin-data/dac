package update

import (
	"fmt"
	"io"
	"os"
)

// Nudge prints a one-line "update available" notice to w if res indicates a
// newer version. Returns true if it printed anything.
//
// Skips when w is a non-terminal *os.File so scripts and CI logs stay clean.
func Nudge(w io.Writer, res Result) bool {
	if !res.HasUpdate {
		return false
	}
	if f, ok := w.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	cmd := "dac upgrade"
	if res.Channel == ChannelEdge {
		cmd = "dac upgrade --channel edge"
	}
	fmt.Fprintf(w, "dac %s available (current: %s). Run `%s`.\n",
		res.LatestVersion, res.CurrentVersion, cmd)
	return true
}
