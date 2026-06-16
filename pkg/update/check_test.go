package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/afero"
)

func newTestChecker(t *testing.T, version string, server *httptest.Server) *Checker {
	t.Helper()
	return &Checker{
		currentVersion: version,
		channel:        detectChannel(version),
		httpClient:     server.Client(),
		fs:             afero.NewMemMapFs(),
		homeDir:        filepath.Join("/test", dacHomeDir),
		now:            time.Now,
		apiBase:        server.URL,
		ttl:            defaultTTL,
	}
}

func TestEnabled(t *testing.T) {
	t.Setenv("DAC_NO_UPDATE_CHECK", "")
	t.Setenv("DO_NOT_TRACK", "")

	cases := []struct {
		version string
		want    bool
	}{
		{"v1.2.3", true},
		{"1.2.3", true},
		{"v0.0.0-edge.42.abc123", true},
		{"dev", false},
		{"", false},
		{"test-local", false},
		{"test-debug-fix", false},
	}
	for _, c := range cases {
		if got := Enabled(c.version); got != c.want {
			t.Errorf("Enabled(%q) = %v, want %v", c.version, got, c.want)
		}
	}

	t.Setenv("DAC_NO_UPDATE_CHECK", "1")
	if Enabled("v1.2.3") {
		t.Error("DAC_NO_UPDATE_CHECK=1 should disable check")
	}
	t.Setenv("DAC_NO_UPDATE_CHECK", "")
	t.Setenv("DO_NOT_TRACK", "1")
	if Enabled("v1.2.3") {
		t.Error("DO_NOT_TRACK=1 should disable check")
	}
}

func TestDetectChannel(t *testing.T) {
	if detectChannel("v1.2.3") != ChannelStable {
		t.Error("stable tag should map to ChannelStable")
	}
	if detectChannel("v0.0.0-edge.42.abc123") != ChannelEdge {
		t.Error("edge tag should map to ChannelEdge")
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v1.2.3", "v1.2.3", false},
		{"v1.2.4", "v1.2.3", true},
		{"1.2.3", "v1.2.3", false},
		{"", "v1.2.3", false},
		{"v1.2.3", "", false},
		{"v0.0.0-edge.42.a", "v0.0.0-edge.41.b", true},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestRunStableFetchAndCache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer srv.Close()

	c := newTestChecker(t, "v1.0.0", srv)

	res, ok := c.run(context.Background())
	if !ok {
		t.Fatal("expected ok=true on first run")
	}
	if res.LatestVersion != "v2.0.0" || !res.HasUpdate || res.Channel != ChannelStable {
		t.Errorf("unexpected result: %+v", res)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("expected 1 HTTP hit, got %d", hits)
	}

	// Second run should be served from cache, no HTTP.
	res2, ok := c.run(context.Background())
	if !ok || res2.LatestVersion != "v2.0.0" {
		t.Errorf("second run should hit cache, got %+v ok=%v", res2, ok)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("expected still 1 HTTP hit, got %d", hits)
	}
}

func TestRunCacheExpires(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_ = json.NewEncoder(w).Encode(map[string]string{"tag_name": "v2.0.0"})
	}))
	defer srv.Close()

	c := newTestChecker(t, "v1.0.0", srv)
	now := time.Now()
	c.now = func() time.Time { return now }

	if _, ok := c.run(context.Background()); !ok {
		t.Fatal("first run failed")
	}

	// Advance past TTL.
	now = now.Add(defaultTTL + time.Minute)
	if _, ok := c.run(context.Background()); !ok {
		t.Fatal("post-expiry run failed")
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Errorf("expected cache to expire and refetch, got %d hits", hits)
	}
}

func TestRunEdgeChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"tag_name": "v1.5.0", "prerelease": false},
			{"tag_name": "v0.0.0-edge.50.abc", "prerelease": true},
			{"tag_name": "v0.0.0-edge.49.def", "prerelease": true},
		})
	}))
	defer srv.Close()

	c := newTestChecker(t, "v0.0.0-edge.42.aaa", srv)
	if c.channel != ChannelEdge {
		t.Fatalf("channel = %s, want edge", c.channel)
	}

	res, ok := c.run(context.Background())
	if !ok {
		t.Fatal("expected ok=true")
	}
	if res.LatestVersion != "v0.0.0-edge.50.abc" {
		t.Errorf("LatestVersion = %s, want v0.0.0-edge.50.abc", res.LatestVersion)
	}
	if !res.HasUpdate {
		t.Error("expected HasUpdate=true")
	}
}

func TestRunNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestChecker(t, "v1.0.0", srv)
	if _, ok := c.run(context.Background()); ok {
		t.Error("expected ok=false on 5xx")
	}
}

func TestNudgeSkipsWhenNoUpdate(t *testing.T) {
	var sb strings.Builder
	if Nudge(&sb, Result{HasUpdate: false}) {
		t.Error("Nudge should return false when HasUpdate=false")
	}
	if sb.Len() != 0 {
		t.Errorf("Nudge wrote unexpected output: %q", sb.String())
	}
}

func TestNudgeWritesToWriter(t *testing.T) {
	var sb strings.Builder
	res := Result{
		LatestVersion:  "v2.0.0",
		CurrentVersion: "v1.0.0",
		Channel:        ChannelStable,
		HasUpdate:      true,
	}
	if !Nudge(&sb, res) {
		t.Fatal("Nudge returned false")
	}
	out := sb.String()
	if !strings.Contains(out, "v2.0.0") || !strings.Contains(out, "v1.0.0") {
		t.Errorf("nudge missing versions: %q", out)
	}
	if !strings.Contains(out, "dac upgrade") {
		t.Errorf("nudge missing upgrade hint: %q", out)
	}
}

func TestNudgeEdgeChannelHint(t *testing.T) {
	var sb strings.Builder
	Nudge(&sb, Result{
		LatestVersion:  "v0.0.0-edge.50.abc",
		CurrentVersion: "v0.0.0-edge.42.aaa",
		Channel:        ChannelEdge,
		HasUpdate:      true,
	})
	if !strings.Contains(sb.String(), "--channel edge") {
		t.Errorf("edge nudge missing channel flag hint: %q", sb.String())
	}
}
