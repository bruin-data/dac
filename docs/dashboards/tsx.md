# TSX Format

TSX dashboards use JSX syntax to define dashboards programmatically. DAC transpiles them with esbuild and executes them at load time in a Go-embedded JavaScript runtime.

## File Naming

TSX dashboards belong in the project's `dashboards/` directory and must use the `.dashboard.tsx` extension:

```text
sales.dashboard.tsx
```

## Basic Example

```tsx
export default (
  <Dashboard name="Simple Dashboard" connection="my_db">
    <Row>
      <Metric
        name="Total Revenue"
        col={4}
        sql="SELECT SUM(amount) AS value FROM sales"
        value={{ field: "value", type: "number", format: "$,.2f" }}
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
        x={{ field: "month" }}
        y={{ field: ["revenue"] }}
      />
    </Row>
  </Dashboard>
)
```

## Why TSX

TSX is useful when you need:
- loops and conditionals
- shared variables and helpers
- load-time queries to shape the dashboard
- reusable modules and includes

## Global Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `query` | `query(connection: string, sql: string): QueryResult` | Execute SQL at dashboard load time |
| `include` | `include(path: string): string` | Read a file relative to the dashboard |
| `require` | `require(path: string): any` | Import another module |

### QueryResult

```typescript
interface QueryResult {
  columns: Array<{ name: string; type: string }>
  rows: Array<Array<any>>
}
```

## JSX Components

### Dashboard

```tsx
<Dashboard
  name="Dashboard Name"
  description="Description"
  connection="my_db"
  model="sales"
>
  {children}
</Dashboard>
```

### Row

```tsx
<Row height="400px">
  {widgets}
</Row>
```

### Tabs and Tab

```tsx
<Tabs>
  <Tab name="Overview">
    <Row>{widgets}</Row>
  </Tab>
</Tabs>
```

### Filter

```tsx
<Filter
  name="region"
  type="select"
  default="All"
  options={{ values: ["All", "North America", "Europe", "APAC"] }}
/>
```

### Query

SQL named query:

```tsx
<Query
  name="totalRevenue"
  sql="SELECT SUM(amount) AS value FROM sales"
/>
```

Semantic named query:

```tsx
<Query
  name="onlineByRegion"
  model="sales"
  dimensions={[{ name: "region" }]}
  metrics={["revenue"]}
  segments={["online"]}
  sort={[{ name: "revenue", direction: "desc" }]}
  limit={8}
/>
```

`<Query />` is a declaration node in the dashboard DSL. It defines a reusable query; it does not render anything in the UI.

### Semantic Widgets

Semantic models are defined separately in `semantic/*.yml`. TSX dashboards reference them by name:

```tsx
export default (
  <Dashboard name="Semantic Sales" connection="local_duckdb" model="sales">
    <Filter name="region" type="select" default="North America" options={{ values: ["North America", "Europe", "APAC"] }} />

    <Row>
      <Metric
        name="Revenue"
        metric="revenue"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
        ]}
        col={3}
      />

      <Chart
        name="Revenue Trend"
        chart="area"
        dimension="created_at"
        granularity="month"
        metrics={["revenue"]}
        sort={[{ name: "created_at", direction: "asc" }]}
        col={9}
      />
    </Row>
  </Dashboard>
)
```

The backend compiles semantic widgets and semantic named queries to SQL at request time.

For a complete runnable project, see `examples/semantic-tsx`.

## Widget Components

All widget components accept the same props as their YAML equivalents.

```tsx
<Metric name="Revenue" col={4} sql="..." value={{ field: "value", type: "number", format: "$,.2f" }} />
<Chart name="Trend" chart="area" col={8} sql="..." x={{ field: "month" }} y={{ field: ["revenue"] }} />
<Table name="Orders" col={12} sql="..." />
<Text name="Note" col={6} content="**Important:** This data updates daily." />
<Divider name="sep" col={12} />
<Image name="logo" col={3} src="https://example.com/logo.png" alt="Logo" />
```

## How It Works

DAC:

1. transpiles the TSX file with esbuild
2. executes the resulting JavaScript in goja
3. extracts the default export
4. converts it to the same dashboard model used by YAML

TSX dashboards are fully compatible with filters, named queries, external semantic models, validation, static builds, and Slides export.
