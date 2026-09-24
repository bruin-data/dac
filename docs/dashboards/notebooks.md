# Notebooks

Notebooks add human context to dashboard data in Bruin Cloud. A notebook can
apply to the whole dashboard, a widget, a table row, or a chart point, depending
on the dimension values selected when its content is written.

Dashboard YAML declares reusable notebook definitions. Content is written in
the dashboard UI and is not stored in YAML.

```yaml
name: Revenue overview
connection: warehouse

notebooks:
  - id: revenue_context
    dimensions:
      - name: region
        required: true
      - name: channel
        multiselect: true

rows:
  - widgets:
      - id: revenue_by_region
        name: Revenue by region
        type: table
        query: revenue_by_region
        notebooks:
          - revenue_context
```

Each definition requires a unique `id` and a `dimensions` list. Dimension
values are optional and single-select by default. Set `required: true` when a
value must be selected and `multiselect: true` to allow multiple values.
Use `dimensions: []` for a dashboard-wide definition.

Widgets can also reference notebook definitions from their resolved semantic
model:

```yaml
# semantic/sales.yml
name: sales
source:
  table: sales

dimensions:
  - name: region
    type: string
    expression: region

notebooks:
  - id: regional_context
    dimensions:
      - name: region
```

```yaml
# dashboards/revenue.yml
name: Revenue
model: sales

rows:
  - widgets:
      - id: revenue_by_region
        name: Revenue by region
        type: table
        dimensions: [{ name: region }]
        metrics: [revenue]
        notebooks: [regional_context]
```

The model is resolved from the named query, widget, or dashboard-level `model`,
including aliases. Without a resolved model, only dashboard-level notebook
definitions are available.
