# DAC

DAC is a Dashboard-as-Code tool for defining, validating, and serving dashboards from YAML and TSX.
- Dynamic charts, tabs, loops and conditionals with TSX.
- Supports all the major databases: Postgres, MySQL, Snowflake, BigQuery, Redshift, Databricks, and more via [Bruin](https://github.com/bruin-data/bruin)
- Built-in semantic layer: define metrics and dimensions once in `semantic/`, reference them from any widget. DAC generates the SQL.

It is built for AI agents to build dashboards in a reliable and reviewable way.

![DAC dashboard demo](resources/dac_optimized.gif)

<table>
<thead>
<tr>
<th>TSX</th>
<th>YAML</th>
</tr>
</thead>
<tbody>
<tr>
<td>

<pre lang="tsx"><code>export default (
  &lt;Dashboard name="Simple Dashboard" connection="my_db"&gt;
    &lt;Row&gt;
      &lt;Metric
        name="Total Revenue"
        col={4}
        sql="SELECT SUM(amount) AS value FROM sales"
        value={{ field: "value", type: "number", format: "$,.2f" }}
      /&gt;
    &lt;/Row&gt;
  &lt;/Dashboard&gt;
)</code></pre>

</td>
<td>

<pre lang="yaml"><code>name: Sales Overview
connection: warehouse

rows:
  - widgets:
      - name: Revenue
        type: metric
        sql: SELECT SUM(amount) AS value FROM sales
        value:
          field: value
          type: number
          format: "$,.2f"
        col: 4</code></pre>

</td>
</tr>
</tbody>
</table>

## Install

Install the latest stable DAC release:

```bash
curl -LsSf https://getbruin.com/install/dac | sh
```

Install the latest edge build from `main`:

```bash
curl -LsSf https://getbruin.com/install/dac | sh -s -- --channel edge
```

DAC uses your existing Bruin connections and currently shells out to `bruin query` for query execution. The install script installs the Bruin CLI first when `bruin` is not already available on your `PATH`.

## Quickstart

Create a new starter project:

```bash
dac init my-dashboards
cd my-dashboards
dac validate --dir .
dac validate --dir . --with-database
dac serve --dir . --open
```

The starter includes a SQL-backed YAML dashboard, a semantic YAML dashboard, and a semantic model under `semantic/`.

`dac init` also installs DAC's bundled dashboard authoring skill for Claude and Codex:

```bash
ls .claude/skills/create-dashboard/SKILL.md
ls .codex/skills/create-dashboard
```

For existing projects, run `dac skills install --dir .`.

If you cloned the repository and have `dac` installed, you can also run one of the bundled example projects:

```bash
dac serve --dir examples/basic-yaml
```

## Examples

The repository includes four self-contained example projects under [`examples/`](examples):

| Example | What it shows |
| --- | --- |
| [`examples/basic-yaml`](examples/basic-yaml) | Standard YAML dashboards with filters, SQL queries, and a self-contained Vega-Lite layered-chart demo. |
| [`examples/basic-tsx`](examples/basic-tsx) | A TSX dashboard that uses load-time queries to generate layout from the database. |
| [`examples/semantic-yaml`](examples/semantic-yaml) | A YAML dashboard that reads semantic models from `semantic/` and compiles widgets in the backend. |
| [`examples/semantic-tsx`](examples/semantic-tsx) | A TSX dashboard using external semantic models and backend semantic query compilation. |

## Project Layout

```text
.
├── cmd/         CLI entrypoints
├── pkg/         Dashboard loading, semantic engine, server, query backends
├── frontend/    React frontend embedded into the DAC binary
├── docs/        VitePress documentation source
├── examples/    Runnable example projects for YAML, TSX, and semantic dashboards
├── resources/   README and documentation assets
└── testdata/    Internal fixtures used by tests
```

## Development

```bash
make deps
make test
make build
make dev
```

The main development commands are defined in the [`Makefile`](Makefile). Use `make` targets rather than ad-hoc `go build` or `npm run build` commands so frontend embedding and build flags stay consistent.

## Telemetry

DAC sends anonymous usage events to help us understand which commands are used and where they fail. Each event includes the command name, run duration, OS/architecture, DAC version, and an anonymous install ID stored at `~/.dac/telemetry.json`.

We do not collect:

- SQL queries, query results, or row counts
- Dashboard or widget contents, names, or file paths
- Connection names, hosts, credentials, project IDs, or dataset names
- Any environment variables or shell history

To disable telemetry, set either of these environment variables:

```bash
export TELEMETRY_OPTOUT=1
# or the industry-standard:
export DO_NOT_TRACK=1
```

Builds without a telemetry write key (the default for `make build`) are silent and send nothing.

## Documentation

- Docs source: [`docs/`](docs)
- Example projects: [`examples/`](examples)
- Contribution guide: [`CONTRIBUTING.md`](CONTRIBUTING.md)
- Security policy: [`SECURITY.md`](SECURITY.md)

## License

AGPL-3.0-only. See [`LICENSE`](LICENSE).
