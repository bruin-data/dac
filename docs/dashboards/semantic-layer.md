# Semantic Layer

DAC loads semantic models from a project-level `semantic/` directory. Dashboard files reference those models by name, and the backend compiles semantic widget definitions into SQL at request time.

This keeps dashboard files focused on business intent:

- which dimensions to group by
- which metrics to show
- which segments and filters to apply
- how the result should be rendered

The dashboard author does not need to hand-write the generated SQL.

## Project Layout

```text
my-project/
├── .bruin.yml
├── dashboards/
│   ├── sales.yml
│   └── sales.dashboard.tsx
└── semantic/
    └── sales.yml
```

Each semantic model is a separate `.yml` file under `semantic/`. Dashboard files still live under `dashboards/`.

Run the bundled examples from the repository root:

```shell
dac serve --dir examples/semantic-yaml
dac serve --dir examples/semantic-tsx
```

## Model Files

Example `semantic/sales.yml`:

```yaml
name: sales
label: Sales
description: Semantic model over the sales table

source:
  table: sales

dimensions:
  - name: created_at
    type: time
    granularities:
      day: date_trunc('day', created_at)
      month: date_trunc('month', created_at)
  - name: region
    type: string
  - name: channel
    type: string

metrics:
  - name: revenue
    expression: sum(amount)
    format:
      type: currency
      currency: USD
      decimals: 0
  - name: sales_count
    expression: count(*)
  - name: avg_sale_value
    expression: "{revenue} / {sales_count}"
  - name: online_revenue
    expression: sum(amount)
    filter: "channel = 'online'"

segments:
  - name: online
    filter: "channel = 'online'"
```

## Model Reference

### Top Level

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema` | string | No | Optional schema ID. Defaults to `https://getbruin.com/schemas/dac/semantic-model/v1` when omitted |
| `name` | string | Yes | Model name used by dashboards |
| `label` | string | No | Display label |
| `description` | string | No | Model description |
| `source.table` | string | Yes | Base SQL table or relation |
| `primary_key` | string | No | Key column used as the join target; required on models referenced by another model's `joins` |
| `joins` | array | No | Relationships to other semantic models (see [Joins](#joins)) |
| `dimensions` | array | No | Fields available for grouping and filtering |
| `metrics` | array | No | Aggregated or derived values |
| `segments` | array | No | Reusable SQL predicates |

### Dimensions

Dimensions are fields that dashboards can group by, filter by, or sort by.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Dimension name |
| `type` | string | `string`, `number`, `boolean`, or `time` |
| `expression` | string | SQL expression. Defaults to the dimension name when omitted |
| `granularities` | map | Time bucket expressions such as `day`, `month`, or `year` |
| `label` | string | Optional display label |
| `description` | string | Optional description |
| `hidden` | bool | Hide from UI consumers |
| `group` | string | Optional grouping label |

### Metrics

Metrics are aggregate expressions. They come in three flavors:

- **Base** — raw SQL with aggregation (e.g. `sum(amount)`).
- **Derived** — references other metrics with `{metric_name}` (e.g. `{revenue} / {sales_count}`).
- **Window** — references exactly one metric and applies a window function via the `window` block.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Metric name |
| `expression` | string | SQL aggregate or derived expression |
| `filter` | string | Optional SQL predicate applied to only this metric |
| `format.type` | string | `number`, `currency`, `percentage`, or `decimal` |
| `format.currency` | string | Currency code for currency values |
| `format.decimals` | integer | Decimal precision |
| `window.type` | string | `running_total`, `lag`, `lead`, `rank`, or `percent_of_total` |
| `window.order_by` | string | Dimension to order the window by |
| `window.partition_by` | string[] | Dimensions to partition the window by |
| `window.offset` | integer | Offset for `lag` or `lead` |
| `label` | string | Optional display label |
| `description` | string | Optional description |
| `hidden` | bool | Hide from UI consumers |
| `group` | string | Optional grouping label |

#### Expression Rules

- A metric expression may mix `{ref}` placeholders with raw aggregation, e.g. `sum(amount) / {order_count}`. Naming the aggregation as its own base metric (`revenue: sum(amount)`) is usually clearer, but not required.
- A window metric's `expression` must be exactly a single `{ref}` (e.g. `"{revenue}"`). Apply any further arithmetic in a separate derived metric that references the window metric.
- A window metric cannot transitively depend on a metric that mixes `{refs}` with raw aggregation. The wrapped-query rewrite cannot lift unnamed aggregations into the inner subquery, so any aggregation reachable from a window metric must be a named base metric. Split the offending metric into a base + derived pair.
- For `running_total`, `lag`, `lead`, and `rank`, `window.order_by` is required. Both `window.order_by` and every entry in `window.partition_by` must reference dimensions defined on the same model.

### Segments

Segments are named SQL predicates reused by dashboards.

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Segment name |
| `filter` | string | SQL predicate |
| `label` | string | Optional display label |
| `description` | string | Optional description |

## Joins

A model can declare relationships to other models with a `joins` block. Once joined, a dashboard query can group, filter, or sort by dimensions from the joined model using a `relation.dimension` reference. The join target model must define a `primary_key`.

```yaml
# semantic/orders.yml
name: orders
source:
  table: public.orders
primary_key: order_id

joins:
  - name: customers          # relation name; also the target model name unless `model:` is set
    relationship: many_to_one
    foreign_key: customer_id # column on this model that points at customers.primary_key

dimensions:
  - name: category
    type: string
metrics:
  - name: revenue
    expression: sum(amount)
```

```yaml
# semantic/customers.yml
name: customers
source:
  table: public.customers
primary_key: customer_id
dimensions:
  - name: country
    type: string
```

A widget or named query on the `orders` model can then reference `customers.country`:

```yaml
- name: Revenue by Country
  type: chart
  chart: bar
  dimension: customers.country   # dimension from the joined customers model
  metrics: [revenue]
```

DAC compiles this into a join, e.g.:

```sql
SELECT customers.country AS customers_country, sum(base.amount) AS revenue
FROM (SELECT * FROM public.orders) base
LEFT JOIN (SELECT * FROM public.customers) customers
  ON base.customer_id = customers.customer_id
GROUP BY 1
```

| Join field | Type | Description |
|------------|------|-------------|
| `name` | string | Relation name used to qualify dimensions (`name.dimension`); defaults to the target model name |
| `model` | string | Target model name, if it differs from `name` |
| `relationship` | string | `one_to_one`, `many_to_one`, `one_to_many`, or `many_to_many` |
| `foreign_key` | string | Column on this model that references the target's `primary_key` |
| `target_key` | string | Override for the target column (defaults to the target's `primary_key`) |
| `sql` | string | Custom join condition, used instead of `foreign_key`/`target_key` |

See `examples/semantic-joins` for a complete project.

## Referencing Models

Set a default model for the whole dashboard:

```yaml
name: Semantic Sales Example
connection: local_duckdb
model: sales
```

Widgets and named queries then inherit `sales` unless they set their own `model`.

You can also define aliases with `models`. This is useful when a dashboard references multiple models or when you want stable dashboard-facing names:

```yaml
name: Executive Sales
connection: warehouse
model: sales_model

models:
  sales_model: sales
  support_model: support_tickets
```

In this example, dashboard widgets can use `model: sales_model`, and DAC resolves it to the `sales` semantic model.

## Semantic Widgets

### Metric

```yaml
- name: Revenue
  type: metric
  metric: revenue
  filters:
    - dimension: region
      operator: equals
      value: "{{ filters.region }}"
  value:
    field: revenue
    type: number
    format: "$,.0f"
  col: 3
```

### Chart

```yaml
- name: Revenue Trend
  type: chart
  chart: area
  dimension: created_at
  granularity: month
  metrics: [revenue]
  sort:
    - name: created_at
      direction: asc
  col: 8
```

### Table

```yaml
- name: Sales Breakdown
  type: table
  dimensions:
    - name: region
    - name: channel
  metrics: [revenue, sales_count]
  sort:
    - name: revenue
      direction: desc
  limit: 20
  col: 12
```

## Named Semantic Queries

Named semantic queries live in the dashboard `queries` map and can be reused by widgets:

```yaml
queries:
  online_by_region:
    model: sales
    dimensions:
      - name: region
    metrics: [revenue]
    segments: [online]
    sort:
      - name: revenue
        direction: desc
    limit: 8

rows:
  - widgets:
      - name: Online Revenue by Region
        type: chart
        chart: bar
        query: online_by_region
        col: 4
```

## Filters

DAC has two related filter concepts:

- Dashboard filters define UI controls, such as `select`, `date-range`, `date`, `number`, and `text`.
- Semantic query filters define predicates applied to semantic dimensions before SQL execution.

Dashboard filter values can be referenced in semantic filters with the same Jinja-style syntax used by SQL queries:

```yaml
filters:
  - name: date_range
    type: date-range
    default: all_time

rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
        filters:
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
```

Structured semantic filters target dimensions. Metric-level conditions should usually be modeled as metric `filter` predicates or reusable `segments`.

Supported operators:

| Operator | Value shape |
|----------|-------------|
| `equals` | scalar |
| `not_equals` | scalar |
| `gt` | scalar |
| `gte` | scalar |
| `lt` | scalar |
| `lte` | scalar |
| `in` | array |
| `not_in` | array |
| `between` | `{start, end}` |
| `is_null` | no value |
| `is_not_null` | no value |

Filter values can be strings, numbers, booleans, arrays, or objects. YAML numeric literals remain numeric when they are not wrapped in quotes.

For advanced cases, use an expression filter:

```yaml
filters:
  - expression: "{revenue} > 100000"
```

Expression filters can reference metrics with `{metric_name}` and are compiled into the appropriate SQL predicate.

## TSX Dashboards

TSX dashboards use the same semantic fields as YAML dashboards:

```tsx
export default (
  <Dashboard name="Semantic Sales" connection="local_duckdb" model="sales">
    <Filter
      name="region"
      type="select"
      default="North America"
      options={{ values: ["North America", "Europe", "APAC"] }}
    />

    <Query
      name="onlineByRegion"
      model="sales"
      dimensions={[{ name: "region" }]}
      metrics={["revenue"]}
      segments={["online"]}
      sort={[{ name: "revenue", direction: "desc" }]}
      limit={8}
    />

    <Row>
      <Metric
        name="Revenue"
        metric="revenue"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
        ]}
        col={4}
      />
      <Chart
        name="Online Revenue by Region"
        chart="bar"
        query="onlineByRegion"
        col={8}
      />
    </Row>
  </Dashboard>
)
```

`<Query />` is a declaration node in the dashboard DSL. It defines a named query; it does not render a UI component.

## Backend Compilation

Semantic widgets and named semantic queries go through the backend REST API. The backend:

1. resolves the dashboard model or model alias
2. renders templated filter values
3. validates referenced dimensions, metrics, segments, filters, and sort fields
4. compiles the semantic query to SQL
5. executes the generated SQL against the selected connection

SQL generation happens in the application backend, not in YAML or TSX dashboard files.

## Validation

`dac validate` checks semantic model structure and semantic references:

```shell
dac validate --dir examples/semantic-yaml
```

It fails dashboards that reference invalid semantic models, missing metrics, missing dimensions, unknown segments, or malformed filters. Regular SQL dashboards are still valid when they do not reference the broken semantic model.

To inspect a compiled semantic widget by executing it:

```shell
dac query --dir examples/semantic-yaml --dashboard "Semantic Sales Example" --widget "Revenue"
```
