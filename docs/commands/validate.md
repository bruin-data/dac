# dac validate

Validate dashboard definitions without executing any queries by default. Catches structural and configuration errors before you run the server.

```shell
dac validate [dir] [flags]
```

## Flags

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--dir` | `-d` | string | `.` | Dashboard definitions directory |
| `--with-database` | | bool | `false` | Dry-run widget queries against configured database connections |

## Examples

```shell
# Validate all dashboards in current directory
dac validate

# Validate dashboards in a specific directory
dac validate --dir ./dashboards

# Equivalent positional form
dac validate ./dashboards

# Validate dashboards and dry-run every data query
dac validate --with-database
```

## What It Checks

- **Dashboard**: `name` is required, at least one row
- **Schemas**: YAML dashboards, semantic models, and themes match the v1 Bruin schema for their file type; explicit `schema` values must be v1
- **Rows**: at least one widget per row
- **Widgets**: `type` and `name` are required
- **Grid**: column spans are 1-12, row totals don't exceed 12
- **Query references**: named queries referenced by widgets exist in the `queries` map
- **Filter types**: must be `select`, `date-range`, `date`, `number`, or `text`
- **Chart types**: valid chart type names
- **Semantic layer**:
  - model files under `semantic/` have a `name` and `source.table`
  - metric expressions are present and derived metric references exist
  - referenced dashboard models or aliases exist
  - semantic widgets reference valid metrics, dimensions, segments, filters, and sort fields
  - invalid semantic models only fail dashboards that reference them

## Database Dry Runs

`--with-database` validates compiled widget SQL against your configured Bruin connections without returning dashboard rows.

DAC uses `bruin query --dry-run` when the Bruin CLI supports it. If a backend only exposes regular execution, DAC falls back to `EXPLAIN <query>`. DAC refuses potentially mutating SQL before dry-running; only read-only `SELECT`, `WITH`, `VALUES`, `SHOW`, `DESCRIBE`, and `EXPLAIN` statements are accepted.

For TSX dashboards, load-time `query()` calls are dry-run validated and return empty rows during validation. Use `dac check` when you need to execute queries and verify returned result shapes.

## Exit Code

- `0` — all dashboards valid
- `1` — validation errors found
