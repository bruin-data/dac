package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/bruin-data/dac/pkg/config"
)

// BruinCLIBackend implements Backend by shelling out to `bruin query`.
type BruinCLIBackend struct {
	BruinPath   string         // Path to bruin binary (default: "bruin" from PATH)
	ConfigFile  string         // Path to .bruin.yml
	Config      *config.Config // Preloaded for one CLI operation; servers leave this nil.
	Environment string         // Target environment
}

// bruinQueryResponse mirrors the JSON output of `bruin query --output json`.
type bruinQueryResponse struct {
	Columns []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows           [][]interface{} `json:"rows"`
	ConnectionName string          `json:"connectionName"`
	Query          string          `json:"query"`
}

type bruinDryRunResponse struct {
	ConnectionName string `json:"connectionName"`
	ConnectionType string `json:"connectionType"`
	Query          string `json:"query"`
	Valid          *bool  `json:"valid"`
	Error          string `json:"error"`
	Columns        []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"columns"`
	Rows [][]interface{} `json:"rows"`
}

func (b *BruinCLIBackend) Execute(ctx context.Context, connection string, sql string) (*QueryResult, error) {
	args := b.queryArgs(connection, sql, false)
	stdout, stderr, err := b.run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("bruin query failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	var resp bruinQueryResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing bruin query output: %w\nraw: %s", err, stdout.String())
	}

	result := &QueryResult{
		Rows: resp.Rows,
	}
	for _, col := range resp.Columns {
		result.Columns = append(result.Columns, ColumnInfo{
			Name: col.Name,
			Type: col.Type,
		})
	}
	return result, nil
}

func (b *BruinCLIBackend) DryRun(ctx context.Context, connection string, sql string) (*DryRunResult, error) {
	args := b.queryArgs(connection, sql, true)
	stdout, stderr, err := b.run(ctx, args)
	if err != nil {
		if msg := bruinJSONError(stdout.Bytes()); msg != "" {
			return nil, fmt.Errorf("bruin dry-run failed: %w: %s", err, msg)
		}
		return nil, fmt.Errorf("bruin dry-run failed: %w\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}

	var resp bruinDryRunResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing bruin dry-run output: %w\nraw: %s", err, stdout.String())
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("bruin dry-run failed: %s", resp.Error)
	}
	if resp.Valid != nil && !*resp.Valid {
		return nil, fmt.Errorf("bruin dry-run reported invalid query")
	}

	result := &DryRunResult{
		Query:          resp.Query,
		ConnectionName: resp.ConnectionName,
		ConnectionType: resp.ConnectionType,
		Valid:          resp.Valid == nil || *resp.Valid,
		Rows:           resp.Rows,
	}
	for _, col := range resp.Columns {
		result.Columns = append(result.Columns, ColumnInfo{
			Name: col.Name,
			Type: col.Type,
		})
	}
	return result, nil
}

func (b *BruinCLIBackend) queryArgs(connection string, sql string, dryRun bool) []string {
	args := []string{"query", "--output", "json"}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if connection != "" {
		args = append(args, "-c", connection)
	}
	if b.ConfigFile != "" {
		args = append(args, "--config-file", b.ConfigFile)
	}
	if b.Environment != "" {
		args = append(args, "-e", b.Environment)
	}
	return append(args, "-q", sql)
}

func (b *BruinCLIBackend) run(ctx context.Context, args []string) (bytes.Buffer, bytes.Buffer, error) {
	bin := b.BruinPath
	if bin == "" {
		bin = "bruin"
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout, stderr, err
}

func bruinJSONError(data []byte) string {
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return ""
	}
	return resp.Error
}
