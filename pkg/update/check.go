// Package update fetches the latest dac release tag from GitHub and caches
// it under ~/.dac/update-check.json so subsequent runs avoid the network
// round-trip. The result drives a one-line "update available" nudge printed
// at the end of a command.
//
// Disable with DAC_NO_UPDATE_CHECK=1 or DO_NOT_TRACK=1.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
)

const (
	owner          = "bruin-data"
	repo           = "dac"
	dacHomeDir     = ".dac"
	cacheFileName  = "update-check.json"
	defaultTTL     = 24 * time.Hour
	defaultTimeout = 3 * time.Second
)

// Channel describes which release stream the current binary tracks.
type Channel string

const (
	ChannelStable Channel = "stable"
	ChannelEdge   Channel = "edge"
)

// Result is what the asynchronous check returns.
type Result struct {
	LatestVersion  string
	CurrentVersion string
	Channel        Channel
	HasUpdate      bool
}

type cacheEntry struct {
	LatestVersion string    `json:"latest_version"`
	Channel       Channel   `json:"channel"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Checker drives an asynchronous update check. Construct with NewChecker.
type Checker struct {
	currentVersion string
	channel        Channel
	httpClient     *http.Client
	fs             afero.Fs
	homeDir        string
	now            func() time.Time
	apiBase        string
	ttl            time.Duration
}

// NewChecker returns a Checker configured for real-world use. Returns nil if
// the current version is not eligible for nudging (dev/test builds, opt-out).
func NewChecker(currentVersion string) *Checker {
	if !Enabled(currentVersion) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return &Checker{
		currentVersion: currentVersion,
		channel:        detectChannel(currentVersion),
		httpClient:     &http.Client{Timeout: defaultTimeout},
		fs:             afero.NewOsFs(),
		homeDir:        filepath.Join(home, dacHomeDir),
		now:            time.Now,
		apiBase:        "https://api.github.com",
		ttl:            defaultTTL,
	}
}

// Enabled reports whether update checks should run for this build.
func Enabled(currentVersion string) bool {
	if os.Getenv("DAC_NO_UPDATE_CHECK") != "" || os.Getenv("DO_NOT_TRACK") != "" {
		return false
	}
	if currentVersion == "" || currentVersion == "dev" {
		return false
	}
	if strings.HasPrefix(currentVersion, "test-") {
		return false
	}
	return true
}

// Start kicks off the check in a goroutine and returns a channel that yields
// a single Result on success. The channel is closed without a value when the
// check is skipped, fails, or times out — receivers should use a select with
// a deadline so they never block the main flow.
func (c *Checker) Start(ctx context.Context) <-chan Result {
	out := make(chan Result, 1)
	go func() {
		defer close(out)
		res, ok := c.run(ctx)
		if ok {
			out <- res
		}
	}()
	return out
}

func (c *Checker) run(ctx context.Context) (Result, bool) {
	cached, fresh := c.readCache()
	var latest string
	var channel Channel
	switch {
	case fresh:
		latest = cached.LatestVersion
		channel = cached.Channel
	default:
		v, ch, err := c.fetchLatest(ctx)
		if err != nil {
			return Result{}, false
		}
		latest, channel = v, ch
		c.writeCache(cacheEntry{LatestVersion: latest, Channel: channel, CheckedAt: c.now().UTC()})
	}

	if latest == "" {
		return Result{}, false
	}
	return Result{
		LatestVersion:  latest,
		CurrentVersion: c.currentVersion,
		Channel:        channel,
		HasUpdate:      isNewer(latest, c.currentVersion),
	}, true
}

func (c *Checker) readCache() (cacheEntry, bool) {
	path := filepath.Join(c.homeDir, cacheFileName)
	buf, err := afero.ReadFile(c.fs, path)
	if err != nil {
		return cacheEntry{}, false
	}
	var entry cacheEntry
	if err := json.Unmarshal(buf, &entry); err != nil {
		return cacheEntry{}, false
	}
	if entry.Channel != c.channel {
		return cacheEntry{}, false
	}
	if c.now().Sub(entry.CheckedAt) > c.ttl {
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *Checker) writeCache(entry cacheEntry) {
	if err := c.fs.MkdirAll(c.homeDir, 0o755); err != nil {
		return
	}
	buf, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_ = afero.WriteFile(c.fs, filepath.Join(c.homeDir, cacheFileName), buf, 0o600)
}

func (c *Checker) fetchLatest(ctx context.Context) (string, Channel, error) {
	switch c.channel {
	case ChannelEdge:
		v, err := c.fetchLatestEdge(ctx)
		return v, ChannelEdge, err
	default:
		v, err := c.fetchLatestStable(ctx)
		return v, ChannelStable, err
	}
}

func (c *Checker) fetchLatestStable(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.apiBase, owner, repo)
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := c.getJSON(ctx, url, &payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

func (c *Checker) fetchLatestEdge(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30", c.apiBase, owner, repo)
	var releases []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := c.getJSON(ctx, url, &releases); err != nil {
		return "", err
	}
	for _, r := range releases {
		if r.Prerelease && strings.HasPrefix(r.TagName, "v0.0.0-edge.") {
			return r.TagName, nil
		}
	}
	return "", nil
}

func (c *Checker) getJSON(ctx context.Context, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("github api: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

func detectChannel(version string) Channel {
	if strings.Contains(version, "-edge.") {
		return ChannelEdge
	}
	return ChannelStable
}

// isNewer reports whether latest is a different tag than current. We do not
// parse semver — GitHub already tells us the latest tag, so any mismatch
// means there's something newer to install. A bare equality check avoids
// edge-tag quirks like "v0.0.0-edge.42.abc123".
func isNewer(latest, current string) bool {
	l := strings.TrimPrefix(latest, "v")
	c := strings.TrimPrefix(current, "v")
	return l != "" && c != "" && l != c
}
