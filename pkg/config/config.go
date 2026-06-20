package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .bruin.yml configuration file.
type Config struct {
	DefaultEnvironment string                 `yaml:"default_environment"`
	Environments       map[string]Environment `yaml:"environments"`
}

type Environment struct {
	SchemaPrefix string                  `yaml:"schema_prefix,omitempty"`
	Connections  map[string][]Connection `yaml:"connections"`
}

// Connection is a generic connection entry. Different connection types have
// different fields, but they all share a name. We use a map for the rest
// since .bruin.yml supports dozens of connection types.
type Connection struct {
	Name string `yaml:"name"`
	// Extra holds all other fields (host, port, database, etc.).
	Extra map[string]any `yaml:",inline"`
}

// Load reads and parses a .bruin.yml file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// Discover walks up from the given directory to find a .bruin.yml file.
func Discover(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		path := filepath.Join(dir, ".bruin.yml")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf(".bruin.yml not found (searched from %s to root)", startDir)
		}
		dir = parent
	}
}

// GetEnvironment returns the named environment, or the default one.
func (c *Config) GetEnvironment(name string) (*Environment, error) {
	if name == "" {
		name = c.DefaultEnvironment
	}
	if name == "" {
		// If there's exactly one environment, use it.
		if len(c.Environments) == 1 {
			for _, env := range c.Environments {
				return &env, nil
			}
		}
		return nil, fmt.Errorf("no environment specified and no default_environment set")
	}

	env, ok := c.Environments[name]
	if !ok {
		return nil, fmt.Errorf("environment %q not found in config", name)
	}
	return &env, nil
}

// FindConnection looks up a connection by name across all connection types in an environment.
func (e *Environment) FindConnection(name string) (*Connection, error) {
	for _, conns := range e.Connections {
		for _, c := range conns {
			if c.Name == name {
				return &c, nil
			}
		}
	}
	return nil, fmt.Errorf("connection %q not found", name)
}

// Save writes the config back to a YAML file at the given path.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
