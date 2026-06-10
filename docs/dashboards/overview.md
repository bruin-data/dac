# Dashboard Overview

DAC projects are organized around a project root. Dashboard files live in `dashboards/`, semantic models live in `semantic/`, and optional shared themes live in `themes/`.

## Recommended Project Layout

```text
my-project/
├── .bruin.yml
├── dashboards/
│   ├── sales.yml
│   └── operations.dashboard.tsx
├── semantic/
│   └── sales.yml
└── themes/
    └── custom.yml
```

## Two Formats

DAC supports two dashboard formats:

### YAML

Best for declarative dashboards with static structure.

```yaml
name: Sales Overview
connection: my_db

rows:
  - widgets:
      - name: Revenue
        type: metric
        sql: SELECT SUM(amount) AS value FROM sales
        value:
          field: value
          type: number
          format: "$,.2f"
        col: 4
```

See [YAML Format](/dashboards/yaml).

### TSX

Best for dashboards that need loops, variables, or load-time queries.

```tsx
const tables = query("my_db", "SELECT table_name FROM information_schema.tables WHERE table_schema = 'main'")

export default (
  <Dashboard name="Auto Explorer" connection="my_db">
    <Tabs>
      {tables.rows.map(([table]) => (
        <Tab name={table}>
          <Row>
            <Metric
              name="Row Count"
              col={4}
              sql={`SELECT COUNT(*) AS value FROM "${table}"`}
              value={{ field: "value", type: "number", format: ",.0f" }}
            />
          </Row>
        </Tab>
      ))}
    </Tabs>
  </Dashboard>
)
```

See [TSX Format](/dashboards/tsx).

## Core Concepts

| Concept | Description |
|---------|-------------|
| [Widgets](/dashboards/widgets) | Metrics, charts, tables, text, images, and dividers |
| [Rows](/dashboards/layout) | Horizontal containers using a 12-column grid |
| [Filters](/dashboards/filters) | Interactive controls injected into SQL and semantic queries |
| [Queries](/dashboards/queries) | Named SQL or semantic queries reusable across widgets |
| [Semantic Layer](/dashboards/semantic-layer) | External semantic models loaded from `semantic/` |
| [Themes](/dashboards/themes) | Visual customization via design tokens |
| [Schemas](/dashboards/schemas) | Versioned YAML contracts for dashboards, semantic models, and themes |

## File Discovery

When `--dir` points at a project root, DAC:
- loads dashboards from `dashboards/`
- loads semantic models from `semantic/`
- loads themes from `themes/` when present

Semantic models are optional. Regular SQL dashboards do not need a `semantic/` directory and continue to validate independently of semantic dashboards.

Dashboard discovery rules:
- `*.yml` and `*.yaml` are parsed as YAML dashboards
- `*.dashboard.tsx` files are parsed as TSX dashboards
- files starting with `.` are ignored

You can also point `--dir` directly at the `dashboards/` folder. DAC will still resolve sibling `semantic/` and `themes/` directories from the parent project when they exist.

```shell
# serve a project from its root
dac serve --dir .

# or point directly at dashboards
dac serve --dir ./dashboards
```

## Dashboard Structure

Each dashboard file contains:

```text
Dashboard
├── name, description, connection
├── model / models (optional semantic defaults)
├── filters (optional)
├── queries (optional)
└── rows
    └── widgets
```

Rows can optionally be grouped into tabs for multi-view dashboards.
