# Commands

DAC provides a set of commands for developing, validating, and deploying dashboards.

## Global Flags

These flags apply to all commands:

| Flag | Alias | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to `.bruin.yml` config file (auto-discovered if not set) |
| `--environment` | `-e` | Target environment name |
| `--debug` | | Enable debug logging |

## Command Reference

| Command | Description |
|---------|-------------|
| [`init`](/commands/init) | Create a new DAC project |
| [`serve`](/commands/serve) | Start development server with live reload |
| [`build`](/commands/build) | Build static dashboard with baked-in query results |
| [`validate`](/commands/validate) | Validate dashboard definitions |
| [`check`](/commands/check) | Validate and execute all queries |
| [`ls`](/commands/ls) | List discovered dashboards |
| [`query`](/commands/query) | Run SQL against a connection |
| [`connections`](/commands/connections) | Test database connections |
| [`skills`](/commands/skills) | List, install, and update DAC agent skills |
| [`import`](/commands/import) | Import dashboards from external tools |
| [`export`](/commands/export) | Export dashboards to external formats |
| [`upgrade`](/commands/upgrade) | Upgrade the DAC CLI in place (alias: `update`) |
