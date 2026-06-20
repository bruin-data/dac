# dac import

Import dashboards from external tools into DAC YAML.

## dac import metabase

Convert a Metabase dashboard into a DAC dashboard file.

```shell
dac import metabase [flags]
```

The importer supports two input paths:

- A saved Metabase dashboard API response from `GET /api/dashboard/:id`
- A live Metabase fetch using `--url`, `--dashboard-id`, and an API key or session token

Native SQL questions are converted to DAC metric, chart, and table widgets. Supported Metabase query-builder/MBQL cards are compiled to SQL when they are built on a Metabase model or a physical database table with available table metadata. Unsupported MBQL cards are converted to explicit text placeholders by default. Use `--strict` to fail instead.

For live imports, DAC also fetches Metabase source cards referenced by `source-card` / `card__ID` and table metadata referenced by physical `source-table` IDs so simple query-builder questions can be converted.

Use `--semantic` to generate DAC semantic model files from explicit Metabase semantic artifacts. In this mode, DAC creates semantic dimensions from Metabase models and semantic metrics only from explicit Metabase metrics. Dashboard-card aggregations such as `sum(revenue)` are not promoted into DAC metrics automatically; those cards stay SQL-backed unless they reference a named Metabase metric. Use `--semantic-strict` to fail instead of falling back to SQL for model-backed cards that cannot be represented semantically.

### Flags

| Flag | Alias | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--input` | `-i` | string | | Path to a Metabase dashboard API response JSON/YAML file |
| `--url` | | string | | Metabase base URL for live import |
| `--dashboard-id` | | string | | Metabase dashboard ID for live import |
| `--api-key` | | string | `METABASE_API_KEY` | Metabase API key |
| `--session-token` | | string | `METABASE_SESSION_TOKEN` | Metabase session token |
| `--connection` | `-c` | string | | DAC connection name to set on the generated dashboard |
| `--output` | `-o` | string | `<dir>/<dashboard-slug>.yml` | Output YAML path; use `-` for stdout |
| `--dir` | `-d` | string | `.` | Dashboard definitions directory used for the default output path |
| `--force` | | boolean | `false` | Overwrite an existing output file |
| `--strict` | | boolean | `false` | Fail on unsupported Metabase cards instead of creating placeholders |
| `--semantic` | | boolean | `false` | Generate DAC semantic model files and convert eligible widgets to semantic references |
| `--semantic-strict` | | boolean | `false` | Enable semantic import and fail when a model-backed card cannot be represented semantically |
| `--semantic-dir` | | string | Sibling `semantic/` directory | Directory for generated semantic model files |

### Examples

Import from a saved API response:

```shell
curl \
  -H "X-API-Key: $METABASE_API_KEY" \
  https://metabase.example.com/api/dashboard/42 \
  -o metabase-dashboard.json

dac import metabase \
  --input metabase-dashboard.json \
  --output dashboards/sales.yml \
  --connection warehouse
```

Import directly from Metabase:

```shell
dac import metabase \
  --url https://metabase.example.com \
  --dashboard-id 42 \
  --api-key "$METABASE_API_KEY" \
  --dir dashboards \
  --connection warehouse
```

Import Metabase models and named metrics into DAC's semantic layer:

```shell
dac import metabase \
  --url https://metabase.example.com \
  --dashboard-id 42 \
  --api-key "$METABASE_API_KEY" \
  --output dashboards/sales.yml \
  --connection warehouse \
  --semantic
```

Write the converted YAML to stdout:

```shell
dac import metabase \
  --input metabase-dashboard.json \
  --output -
```

### Conversion Notes

- Metabase dashboard cards are mapped onto DAC's 12-column grid.
- Metabase card widths and positions are imported, but Metabase `size_y` card heights are not mapped to DAC row heights yet.
- Scalar cards become metric widgets.
- Line, bar, area, combo, pie, funnel, gauge, treemap, scatter, and waterfall cards map to DAC charts when native SQL is available.
- Pivot and map cards are mapped to tables.
- Metabase SQL template variables are rewritten as DAC filter references only when the dashboard has an explicit parameter mapping for that card. Unmapped template variables use their Metabase card defaults when available; optional `[[...]]` clauses without a mapped or default value are omitted.
- Metabase optional SQL markers `[[...]]` are removed and reported as warnings so the generated SQL can be reviewed.
- When Metabase omits result metadata, DAC infers simple widget fields and common metric/date types from SQL aliases.
- Dashboard parameters become DAC filters when present in the Metabase dashboard response, even if they are not connected to every card.
- Parameter mappings on model-backed cards are converted into SQL `WHERE` clauses for supported field targets only when the mapping is explicitly scoped to that Metabase card. Unscoped or stale mappings are ignored to match Metabase dashboard behavior.
- Simple Metabase model-backed questions are converted into SQL over the model's source SQL when they use supported raw-field breakouts, filters, order fields, limits, and aggregations such as `sum`, `avg`, `min`, `max`, `count`, and `count-distinct`.
- Simple Metabase physical-table questions are converted into SQL over the source table when table metadata is available and the card uses supported raw-field breakouts, aggregations, filters, order fields, and limits. Detail-row physical-table cards are imported as table widgets with a default `LIMIT 2000` when Metabase does not provide an explicit limit.
- With `--semantic`, explicit Metabase models generate `semantic/*.yml` model files and their result metadata becomes DAC dimensions.
- With `--semantic`, explicit Metabase metrics generate DAC semantic metrics, including supported metric filters. Unnamed dashboard aggregations are deliberately not inferred as semantic metrics because they do not carry durable metric names.
- Model-backed cards that use explicit Metabase metric references can become semantic widgets with DAC dimensions, metrics, filters, and sort fields.
- Model-backed cards that cannot be represented semantically fall back to SQL by default and emit warnings. `--semantic-strict` turns those fallbacks into errors.
