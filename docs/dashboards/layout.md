# Layout

DAC uses a row-based layout system with a 12-column grid.

## Rows

Every dashboard is a vertical stack of rows. Each row contains one or more widgets arranged horizontally:

```yaml
rows:
  - widgets:
      - name: Widget A
        type: metric
        col: 6
      - name: Widget B
        type: metric
        col: 6

  - widgets:
      - name: Widget C
        type: chart
        col: 12
```

### Row Height

Rows can have a fixed height:

```yaml
rows:
  - height: 400px
    widgets:
      - name: Tall Chart
        type: chart
        chart: line
        col: 12
```

Height accepts CSS values (`400px`, `50vh`) or numbers (interpreted as pixels).

## 12-Column Grid

Widget widths are specified using the `col` field (1-12). Columns in a row should add up to 12:

```yaml
# Three equal columns
- widgets:
    - { name: A, type: metric, col: 4 }
    - { name: B, type: metric, col: 4 }
    - { name: C, type: metric, col: 4 }

# Sidebar layout
- widgets:
    - { name: Main, type: chart, col: 8 }
    - { name: Side, type: chart, col: 4 }

# Full width
- widgets:
    - { name: Wide, type: table, col: 12 }
```

Common patterns:

| Layout | Columns |
|--------|---------|
| 1 column | `col: 12` |
| 2 equal columns | `col: 6` + `col: 6` |
| 3 equal columns | `col: 4` + `col: 4` + `col: 4` |
| 4 equal columns | `col: 3` + `col: 3` + `col: 3` + `col: 3` |
| Main + sidebar | `col: 8` + `col: 4` |
| 5 KPIs | `col: 3` + `col: 2` + `col: 2` + `col: 3` + `col: 2` |

If `col` is omitted, widgets are auto-sized to fill the remaining space equally.

## Tabs

Group rows into tabs for multi-view dashboards:

```yaml
rows:
  # Rows before tabs are always visible
  - widgets:
      - { name: Revenue, type: metric, col: 4 }
      - { name: Orders, type: metric, col: 4 }
      - { name: Customers, type: metric, col: 4 }

  # Tabbed content
  - tab: Overview
    widgets:
      - { name: Revenue Trend, type: chart, chart: area, col: 12 }

  - tab: Overview
    widgets:
      - { name: Top Customers, type: table, col: 12 }

  - tab: Breakdown
    widgets:
      - { name: By Region, type: chart, chart: bar, col: 6 }
      - { name: By Channel, type: chart, chart: bar, col: 6 }
```

Rows with the same `tab` name are grouped together. Rows without a `tab` appear above all tabs.

In TSX, use the `<Tabs>` and `<Tab>` components:

```tsx
<Dashboard name="Analytics" connection="my_db">
  <Row>
    <Metric name="Revenue" col={4} sql="..." value={{ field: "value", type: "number", format: "$,.2f" }} />
  </Row>

  <Tabs>
    <Tab name="Overview">
      <Row>
        <Chart name="Revenue Trend" chart="area" col={12} sql="..." x="month" y={["revenue"]} />
      </Row>
    </Tab>
    <Tab name="Breakdown">
      <Row>
        <Chart name="By Region" chart="bar" col={6} sql="..." x="region" y={["revenue"]} />
        <Chart name="By Channel" chart="bar" col={6} sql="..." x="channel" y={["revenue"]} />
      </Row>
    </Tab>
  </Tabs>
</Dashboard>
```

## Layout Tips

- **KPI rows**: Use 3-5 metrics in a row at `col: 2-4` each
- **Chart rows**: Pair a main chart (`col: 8`) with a breakdown (`col: 4`)
- **Tables**: Usually work best at `col: 12` (full width)
- **Sparklines**: Compact at `col: 4`, great for KPI rows
- Validation will warn if a row's columns exceed 12
