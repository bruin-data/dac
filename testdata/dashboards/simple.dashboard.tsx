// ---------------------------------------------------------------------------
// Data-driven dashboard — queries the database at load time to build itself.
// This is impossible in YAML: the dashboard structure adapts to the data.
// ---------------------------------------------------------------------------

// Query the database at load time to discover regions and statuses.
const regions = query("local_duckdb", "SELECT DISTINCT region FROM sales ORDER BY 1")
const statuses = query("local_duckdb", "SELECT DISTINCT status FROM orders ORDER BY 1")
const tables = query("local_duckdb",
  "SELECT table_name FROM information_schema.tables WHERE table_schema = 'main' ORDER BY 1"
)
const regionSeries = regions.rows.length ? regions.rows.map(([r]) => r) : ["placeholder"]

// Reusable KPI component
function KPI({ name, sql, format = ",.0f", ...rest }) {
  return <Metric name={name} sql={sql} value={{ field: "value", type: "number", format }} {...rest} />
}

export default (
  <Dashboard name="Sales (TSX)" connection="local_duckdb">
    <Filter name="date_range" type="date-range" default="last_90_days" />

    {/* ---------- Auto-generated filter from live query results ---------- */}
    <Filter
      name="region"
      type="select"
      default="All"
      options={{ values: ["All", ...regions.rows.map(r => r[0])] }}
    />

    {/* ---------- Per-region KPIs — one per region found in the DB ---------- */}
    <Row>
      {regions.rows.map(([region]) => (
        <KPI
          name={`${region}`}
          format="$,.0f"
          col={Math.floor(12 / regions.rows.length)}
          sql={`SELECT SUM(amount) as value FROM sales WHERE region = '${region}'`}
        />
      ))}
    </Row>

    {/* ---------- Per-status order counts — adapts if new statuses appear ---------- */}
    <Row>
      {statuses.rows.map(([status]) => (
        <KPI
          name={`${status.charAt(0).toUpperCase() + status.slice(1)} Orders`}
          col={Math.floor(12 / statuses.rows.length)}
          sql={`SELECT COUNT(*) as value FROM orders WHERE status = '${status}'`}
        />
      ))}
    </Row>

    <Row>
      <Chart
        name="Revenue by Region"
        chart="bar"
        stacked={true}
        col={8}
        sql={`
          SELECT
            STRFTIME(DATE_TRUNC('month', created_at), '%Y-%m') AS month,
            ${regions.rows.map(([r]) =>
              `SUM(CASE WHEN region = '${r}' THEN amount ELSE 0 END) AS "${r}"`
            ).join(",\n            ")}
          FROM sales
          GROUP BY 1 ORDER BY 1
        `}
        x="month"
        y={regionSeries}
      />
      <Chart
        name="Orders by Status"
        chart="pie"
        col={4}
        sql="SELECT status, COUNT(*) as count FROM orders GROUP BY 1"
        label="status"
        value="count"
      />
    </Row>

    {/* ---------- Auto-generated table previews for every table in the DB ---------- */}
    {tables.rows.map(([name]) => (
      <Row>
        <Table
          name={`${name}`}
          col={12}
          sql={`SELECT * FROM "${name}" ORDER BY created_at DESC LIMIT 10`}
        />
      </Row>
    ))}
  </Dashboard>
)
