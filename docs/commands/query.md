# dac query

Run SQL queries against configured connections. Supports inline SQL, SQL files, SQL-backed dashboard widgets, and semantic dashboard widgets.

```shell
dac query [SQL] [flags]
```

## Flags

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--connection` | `-c` | string | | Connection name from `.bruin.yml` |
| `--file` | `-f` | string | | Path to `.sql` file |
| `--dashboard` | | string | | Dashboard name (for widget queries) |
| `--widget` | `-w` | string | | Widget name within dashboard (for tabs: `"Widget / Tab"`, a tab name, or a tab id) |
| `--output` | `-o` | string | `table` | Output format: `table`, `json`, `csv` |
| `--dir` | `-d` | string | `.` | Dashboard definitions directory |

## Three Modes

### 1. Inline SQL

```shell
dac query "SELECT * FROM sales LIMIT 10" --connection my_db
```

### 2. SQL File

```shell
dac query --file queries/report.sql --connection my_db
```

### 3. Dashboard Widget

Execute a specific widget's query with its filter defaults. If the widget uses a semantic model, DAC resolves the model from `semantic/` and compiles the widget to SQL before execution:

```shell
dac query --dashboard "Sales Analytics" --widget "Revenue Trend"
```

For a `type: tabs` widget, pick one tab. `--widget` accepts:

| Value | Picks |
|-------|-------|
| `"Sales / Revenue"` | The `Revenue` tab of the `Sales` widget |
| `"Revenue"` | A widget or tab whose full label is `Revenue` (a title-less widget's tab is labelled by its tab name alone); otherwise, as a shortcut, the only tab named `Revenue` |
| `"r0-w1::Revenue"` | The tab by its id (row 0, widget 1), always unique — use it for a title-less widget when names clash |

If the value matches more than one widget or tab, the command fails and lists each match with its id. Passing the name of a tabs widget itself lists its tab names. The value is only matched against names and ids.

## Output Formats

```shell
# Table (default)
dac query "SELECT region, COUNT(*) as n FROM sales GROUP BY 1" -c my_db

# JSON
dac query "SELECT * FROM sales LIMIT 5" -c my_db -o json

# CSV
dac query "SELECT * FROM sales" -c my_db -o csv > export.csv
```

## Examples

```shell
# Quick ad-hoc query
dac query "SELECT COUNT(*) FROM orders" -c my_db

# Test a widget's query in isolation
dac query --dashboard "Sales Analytics" --widget "Total Revenue"

# Export to CSV
dac query --file queries/full_export.sql -c warehouse -o csv > report.csv
```
