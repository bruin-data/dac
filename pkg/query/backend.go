package query

import (
	"context"
	"sync"

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
	return NewConnectionTypeResolver(backends...)(connection)
}

// NewConnectionTypeResolver shares one lazy config read within a resolution batch.
// Create a new resolver for each batch so subsequent requests see config edits.
func NewConnectionTypeResolver(backends ...Backend) func(string) (string, error) {
	environment := sync.OnceValues(func() (*config.Environment, error) {
		if len(backends) == 0 {
			return nil, nil
		}
		backend := backends[0]
		for {
			cached, ok := backend.(*CachedBackend)
			if !ok {
				break
			}
			backend = cached.Backend
		}
		bruin, ok := backend.(*BruinCLIBackend)
		if !ok || bruin.ConfigFile == "" {
			return nil, nil
		}
		cfg := bruin.Config
		if cfg == nil {
			var err error
			cfg, err = config.Load(bruin.ConfigFile)
			if err != nil {
				return nil, err
			}
		}
		return cfg.GetEnvironment(bruin.Environment)
	})
	return func(connection string) (string, error) {
		env, err := environment()
		if err != nil || env == nil {
			return "", err
		}
		conn, err := env.FindConnection(connection)
		if err != nil {
			return "", err
		}
		return conn.Type, nil
	}
}
