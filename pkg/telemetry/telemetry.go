// Package telemetry sends anonymous usage events to RudderStack.
//
// Mirrors the architecture of bruin-data/bruin's pkg/telemetry so improvements
// stay portable between the two CLIs. No PII is captured: only command names,
// durations, OS/arch, version, and an anonymous install ID stored at
// ~/.dac/telemetry.json. Disable with TELEMETRY_OPTOUT=1 or DO_NOT_TRACK=1.
package telemetry

import (
	"context"
	"fmt"
	"io"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
	analytics "github.com/rudderlabs/analytics-go/v4"
	"github.com/urfave/cli/v3"
)

// silentLogger swallows all RudderStack client output so misconfigured builds
// never leak "rudder INFO/ERROR" lines into a user's terminal.
type silentLogger struct{}

func (silentLogger) Logf(string, ...interface{})   {}
func (silentLogger) Errorf(string, ...interface{}) {}

const (
	url          = "https://getbruinbumlky.dataplane.rudderstack.com"
	startTimeKey = "telemetry_start"
)

type contextKey string

var TelemetryKey string
var (
	OptOut       = false
	AppVersion   = ""
	RunID        = ""
	TemplateName = "" // Stores template name for init command (protected by lock)
	client       analytics.Client
	lock         sync.Mutex
	installID    string
)

// SetTemplateName stores the template name picked during `dac init` so the
// AfterCommand hook can attach it to the command_end event.
func SetTemplateName(name string) {
	lock.Lock()
	defer lock.Unlock()
	TemplateName = name
}

func Init() io.Closer {
	c, err := analytics.NewWithConfig(TelemetryKey, analytics.Config{
		DataPlaneUrl: url,
		Logger:       silentLogger{},
	})
	if err != nil {
		// Fall back to a no-op closer; we never want telemetry init to break the CLI.
		return io.NopCloser(nil)
	}
	client = c

	if OptOut || TelemetryKey == "" {
		return client
	}

	state, isNew, err := loadOrCreateInstallState(AppVersion)
	if err != nil {
		log.Printf("telemetry: failed to load install state: %v", err)
		return client
	}

	lock.Lock()
	installID = state.InstallID
	lock.Unlock()

	if isNew {
		SendEvent("install", analytics.Properties{
			"install_id":      state.InstallID,
			"install_at":      state.InstallAt,
			"install_version": state.InstallVersion,
		})
		SendEvent("first_run", analytics.Properties{
			"install_id":      state.InstallID,
			"install_at":      state.InstallAt,
			"install_version": state.InstallVersion,
		})
	}

	return client
}

func SendEvent(event string, properties analytics.Properties) {
	lock.Lock()
	defer lock.Unlock()
	if RunID == "" {
		RunID = uuid.New().String()
	}
	if OptOut || TelemetryKey == "" {
		return
	}

	if properties == nil {
		properties = analytics.Properties{}
	}
	if installID != "" {
		if _, ok := properties["install_id"]; !ok {
			properties["install_id"] = installID
		}
	}
	properties["run_id"] = RunID

	id := installID
	if id == "" {
		id, _ = machineid.ID()
	}
	if id == "" {
		id = RunID
	}

	if client == nil {
		return
	}

	_ = client.Enqueue(analytics.Track{
		AnonymousId:       id,
		Event:             event,
		OriginalTimestamp: time.Now().In(time.UTC),
		Context: &analytics.Context{
			App: analytics.AppInfo{
				Name:    "DAC CLI",
				Version: AppVersion,
			},
			OS: analytics.OSInfo{
				Name: runtime.GOOS + " " + runtime.GOARCH,
			},
		},
		Properties: properties,
	})
}

func BeforeCommand(ctx context.Context, c *cli.Command) (context.Context, error) {
	start := time.Now()
	ctx = context.WithValue(ctx, contextKey(startTimeKey), start)
	SendEvent("command_start", analytics.Properties{
		"command": c.Name,
	})
	return ctx, nil
}

func AfterCommand(ctx context.Context, cmd *cli.Command) error {
	start := ctx.Value(contextKey(startTimeKey))
	durationMs := int64(-1)
	if start != nil {
		durationMs = time.Since(start.(time.Time)).Milliseconds()
	}
	properties := analytics.Properties{
		"command":     cmd.Name,
		"duration_ms": durationMs,
	}

	lock.Lock()
	if TemplateName != "" && cmd.Name == "init" {
		properties["template_name"] = TemplateName
		TemplateName = ""
	}
	lock.Unlock()

	SendEvent("command_end", properties)
	return nil
}

func ErrorCommand(ctx context.Context, cmd *cli.Command, err error) error {
	if err == nil {
		return nil
	}
	fmt.Println(err)
	start := ctx.Value(contextKey(startTimeKey))
	startTime, ok := start.(time.Time)
	if !ok {
		return nil
	}

	SendEvent("command_error", analytics.Properties{
		"command":     cmd.Name,
		"duration_ms": time.Since(startTime).Milliseconds(),
	})
	return nil
}
