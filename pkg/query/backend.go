package query

import (
	"context"

	"github.com/bruin-data/dac/pkg/config"
)

// Backend defines the interface for executing queries against a data source.
type Backend interface {
	Execute(ctx context.Context, connection string, query string) (*QueryResult, error)
}

// DryRunner is implemented by backends that can validate a query without
// returning result rows.
type DryRunner interface {
	DryRun(ctx context.Context, connection string, query string) (*DryRunResult, error)
}

// ConnectionType looks up the configured database type using the existing config loader.
func ConnectionType(connection string, backends ...Backend) (string, error) {
	if len(backends) == 0 {
		return "", nil
	}
	backend := backends[0]
	if cached, ok := backend.(*CachedBackend); ok {
		return ConnectionType(connection, cached.Backend)
	}
	bruin, ok := backend.(*BruinCLIBackend)
	if !ok || bruin.ConfigFile == "" {
		return "", nil
	}
	cfg, err := config.Load(bruin.ConfigFile)
	if err != nil {
		return "", err
	}
	env, err := cfg.GetEnvironment(bruin.Environment)
	if err != nil {
		return "", err
	}
	conn, err := env.FindConnection(connection)
	if err != nil {
		return "", err
	}
	return conn.Type, nil
}
