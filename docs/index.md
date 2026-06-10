# Overview

![DAC](/dac.gif)

DAC (**D**ashboard-**a**s-**C**ode) is a tool for defining, validating, and serving data dashboards from version-controlled source files. 
- Build dashboards in YAML or TSX, executed against your existing [Bruin](https://github.com/bruin-data/bruin) connections, and rendered through an embedded React frontend that ships in a single Go binary
- Supports 17+ chart types, interactive filters, a semantic layer for reusable metrics, and validation tools to catch errors before they reach **production**
- [Semantic layer](/dashboards/semantic-layer). Define metrics and dimensions once in `semantic/`, reference them from any widget. DAC generates the SQL.
- A single binary deployment that can run locally, on a server, or be exported as static HTML for hosting anywhere
- Designed for use by both humans and AI agents, with a focus on reviewable, reproducible, and composable dashboard definitions
- Supports all the major databases: Postgres, MySQL, Snowflake, BigQuery, Redshift, Databricks, and more via [Bruin](https://github.com/bruin-data/bruin)
- Interactive [filters](/dashboards/filters). Date pickers, numeric inputs, dropdowns, multiselects, and search inputs that inject into SQL via [Jinja templating](/dashboards/queries) and re-run affected widgets in place.
- Dynamic charts, tabs, loops and conditionals with TSX

```tsx
export default (
  <Dashboard name="Simple Dashboard" connection="my_db">
    <Row>
      <Metric
        name="Total Revenue"
        col={4}
        sql="SELECT SUM(amount) AS value FROM sales"
        column="value"
        prefix="$"
        format="number"
      />
      <Chart
        name="Revenue Over Time"
        chart="area"
        col={8}
        sql={`
          SELECT
            STRFTIME(DATE_TRUNC('month', created_at), '%Y-%m') AS month,
            SUM(amount) AS revenue
          FROM sales
          GROUP BY 1
          ORDER BY 1
        `}
        x="month"
        y={["revenue"]}
      />
    </Row>
  </Dashboard>
)
```

DAC is meant to be a tool to be used heavily by AI agents, such as Claude Code, Codex, or OpenCode. You can install the DAC skill via `dac skills install` and ask your agents to build you a dashboard. DAC gives your agents an easy way to build and validate their work on the dashboards.


## What is DAC?

A dashboard is a `.yml` or `.dashboard.tsx` file in your repo. You describe widgets, queries, filters, and layout. DAC validates the definition, runs the queries against the connection you've configured, and serves an interactive dashboard at `localhost:8321`.

There is no GUI builder, no visual editor, and no separate dashboard service to operate. The source file is the source of truth — review it in pull requests, deploy it like any other code.

```yaml
name: Sales Overview
connection: warehouse

rows:
  - widgets:
      - name: Revenue
        type: metric
        sql: SELECT SUM(amount) AS value FROM sales
        column: value
        prefix: "$"
        col: 4
```


- **Two authoring formats.** YAML for declarative dashboards. [TSX](/dashboards/tsx) when you need loops, variables, conditionals, or queries that resolve at load time to drive layout.
- **21 chart types.** Line, bar, area, pie, scatter, bubble, combo, histogram, boxplot, funnel, sankey, heatmap, calendar, sparkline, waterfall, XMR, dumbbell, gauge, treemap, radar, candlestick — plus metrics, tables, text, images, and dividers.
- **[Semantic layer](/dashboards/semantic-layer).** Define metrics and dimensions once in `semantic/`, reference them from any widget. DAC generates the SQL.
- **Live reload.** Edit the file, save, see the change. No restart, no rebuild.
- **Static export.** `dac build` produces self-contained HTML with query results baked in. Deploy to S3, GitHub Pages, anywhere — no runtime server needed.
- **[Google Slides export](/commands/export).** Render dashboards as slide decks with charts as images and data baked in.
- **Validation in CI.** `dac validate` and `dac check` catch broken queries, missing columns, and schema violations before they reach production.

## Why DAC Exists

Existing dashboarding tools — Looker, Metabase, Tableau, Mode — make dashboards a database row. They live in a hosted service, get edited through a UI, and can't be reviewed, diffed, or rolled back like code. Multiple people clicking through the same UI produces drift you can't see until something breaks.

DAC treats dashboards as plain text:

- **Diffable.** Every change shows up in `git diff`. PR review actually works.
- **Reproducible.** The same source file produces the same dashboard on every machine. No "works in my workspace."
- **Composable.** Define a metric once in the semantic layer, reuse it across dashboards. Loop over a list of tables in TSX to generate dozens of similar views from one definition.
- **Portable.** One Go binary. Connections come from `.bruin.yml`, which you already have if you use Bruin pipelines. Static export means dashboards can live anywhere a static site can.
- **Honest.** What you see in the file is what runs. There is no hidden state in a service somewhere.

If your data pipelines, models, and tests live in version control, your dashboards should too.

## Next Steps

- [Install DAC](/getting-started/installation) and run the [quickstart](/getting-started/quickstart).
- Read the [Dashboard Overview](/dashboards/overview) for the full authoring model.
- Browse the [command reference](/commands/overview).
