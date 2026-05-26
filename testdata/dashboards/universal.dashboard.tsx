// ---------------------------------------------------------------------------
// Universal Self-Building Dashboard
// ---------------------------------------------------------------------------
// Drop this single file next to ANY database. It introspects the schema,
// classifies every column, and generates an entire analytical dashboard:
//   - KPIs from numeric columns
//   - Time-series charts from date + numeric combos
//   - Breakdowns from low-cardinality categorical columns
//   - Data preview tables for every table
//
// This is impossible in any BI tool — the dashboard structure itself is
// generated from the data. Add a table to your DB, refresh, and the
// dashboard grows automatically. No config. No drag-and-drop.
// ---------------------------------------------------------------------------

const conn = "local_duckdb"

// ─── Step 1: Discover all tables ────────────────────────────────────────────
const tablesResult = query(conn,
  `SELECT table_name
   FROM information_schema.tables
   WHERE table_schema = 'main' AND table_type = 'BASE TABLE'
   ORDER BY table_name`
)
const tableNames = tablesResult.rows.map(r => r[0])

// ─── Step 2: For each table, discover columns and their types ───────────────
const columnsResult = query(conn,
  `SELECT table_name, column_name, data_type, ordinal_position
   FROM information_schema.columns
   WHERE table_schema = 'main'
   ORDER BY table_name, ordinal_position`
)

// Build a schema map: { tableName: [{ name, type, position }] }
const schema = {}
for (const [table, col, dtype, pos] of columnsResult.rows) {
  if (!schema[table]) schema[table] = []
  schema[table].push({ name: col, type: dtype, position: pos })
}

// ─── Step 3: Classify columns ───────────────────────────────────────────────
function isDateType(t)    { return /date|timestamp|time/i.test(t) }
function isNumericType(t) { return /int|float|double|decimal|numeric|real|bigint|smallint|tinyint|hugeint/i.test(t) }
function isBoolType(t)    { return /bool/i.test(t) }
function isIdColumn(name) { return /^id$|_id$/i.test(name) }

// For each table, find key column roles
function analyzeTable(tableName) {
  const cols = schema[tableName] || []
  const dateCols = cols.filter(c => isDateType(c.type) && !isIdColumn(c.name))
  const numericCols = cols.filter(c => isNumericType(c.type) && !isIdColumn(c.name))
  const boolCols = cols.filter(c => isBoolType(c.type))
  const textCols = cols.filter(c => !isDateType(c.type) && !isNumericType(c.type) && !isBoolType(c.type) && !isIdColumn(c.name))

  // Prefer a date column named "created_at", "date", etc. for time axis
  const timeCol = dateCols.find(c => /created|date|time|day|month/i.test(c.name)) || dateCols[0]

  // "Amount-like" numeric columns are more interesting for KPIs
  const valueCols = numericCols.filter(c => /amount|revenue|price|cost|budget|salary|spend|mrr|count|score|quantity|sessions|pageviews|clicks|impressions|conversions|hours|usage/i.test(c.name))
  const kpiCols = valueCols.length > 0 ? valueCols : numericCols.slice(0, 3)

  return { cols, dateCols, numericCols, boolCols, textCols, timeCol, kpiCols }
}

// ─── Step 4: Discover categorical columns (low cardinality) ─────────────────
// Query cardinalities for text columns to find good filter/breakdown candidates
const textColsList = []
for (const table of tableNames) {
  const { textCols } = analyzeTable(table)
  for (const col of textCols.slice(0, 4)) { // max 4 text cols per table
    textColsList.push([table, col.name])
  }
}

// Batch cardinality check — find columns with <= 20 distinct values
const cardinalityChecks = textColsList.map(([table, col]) =>
  `SELECT '${table}' as t, '${col}' as c, COUNT(DISTINCT "${col}") as card FROM "${table}"`
).join(" UNION ALL ")

const cardResult = cardinalityChecks
  ? query(conn, cardinalityChecks)
  : { rows: [] }

const categoricals = {} // { "table.col": cardinality }
for (const [table, col, card] of cardResult.rows) {
  if (card > 1 && card <= 20) {
    if (!categoricals[table]) categoricals[table] = []
    categoricals[table].push({ name: col, cardinality: card })
  }
}

// ─── Step 5: Get row counts for all tables ──────────────────────────────────
const countQueries = tableNames.map(t =>
  `SELECT '${t}' as table_name, COUNT(*) as row_count FROM "${t}"`
).join(" UNION ALL ")
const countsResult = countQueries
  ? query(conn, countQueries + " ORDER BY row_count DESC")
  : { rows: [] }
const rowCounts = {}
for (const [t, c] of countsResult.rows) rowCounts[t] = c

// ─── Helpers ────────────────────────────────────────────────────────────────
function titleCase(s) {
  return s.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

function formatColumn(col) {
  if (/amount|revenue|price|cost|budget|salary|spend|mrr/i.test(col)) return "currency"
  if (/rate|pct|percent/i.test(col)) return "percent"
  return "number"
}

function bestAggregate(colName) {
  if (/count|quantity|sessions|pageviews|clicks|impressions|conversions|head_count/i.test(colName)) return "SUM"
  if (/rate|score|pct|percent/i.test(colName)) return "AVG"
  if (/amount|revenue|price|cost|budget|salary|spend|mrr/i.test(colName)) return "SUM"
  return "SUM"
}

// Sort tables: largest first, but cap at 8 for the main dashboard sections
const sortedTables = tableNames.sort((a, b) => (rowCounts[b] || 0) - (rowCounts[a] || 0))
const mainTables = sortedTables.slice(0, 8)
const totalRows = Object.values(rowCounts).reduce((a, b) => a + b, 0)
const largestTable = sortedTables[0] || "none"
const smallestTable = sortedTables[sortedTables.length - 1] || "none"
const tabOrder = sortedTables

// ─── BUILD THE DASHBOARD ────────────────────────────────────────────────────

export default (
  <Dashboard name="Universal Explorer" connection={conn} description={`Auto-generated dashbaord from ${tableNames.length} tables, ${totalRows.toLocaleString()} total rows`}>

    {/* ── Global overview KPIs — always visible above tabs ── */}
    <Row>
      <Metric
        name="Tables Discovered"
        col={3}
        sql={`SELECT ${tableNames.length} as value`}
        value={{ field: "value", type: "number", format: ",.0f" }}
      />
      <Metric
        name="Total Rows"
        col={3}
        sql={`SELECT ${totalRows} as value`}
        value={{ field: "value", type: "number", format: ",.0f" }}
      />
      <Metric
        name="Largest Table"
        col={3}
        sql={`SELECT '${largestTable}' as value`}
        value={{ field: "value" }}
      />
      <Metric
        name={`${titleCase(largestTable)} Rows`}
        col={3}
        sql={`SELECT ${rowCounts[largestTable] || 0} as value`}
        value={{ field: "value", type: "number", format: ",.0f" }}
      />
    </Row>

    {/* ── One tab per table — auto-generated from schema ── */}
    <Tabs>
      {tabOrder.map(table => {
        const { cols, timeCol, kpiCols, textCols } = analyzeTable(table)
        const cats = categoricals[table] || []
        const displayCols = cols.slice(0, 10)
        const orderCol = timeCol ? `"${timeCol.name}" DESC` : `1 DESC`

        return (
          <Tab name={titleCase(table)}>
            {/* KPIs for this table */}
            <Row>
              <Metric
                name="Row Count"
                col={3}
                sql={`SELECT COUNT(*) as value FROM "${table}"`}
                value={{ field: "value", type: "number", format: ",.0f" }}
              />
              {kpiCols.slice(0, 3).map(col => {
                const agg = bestAggregate(col.name)
                const fmt = formatColumn(col.name)
                const spec = fmt === "currency" ? "$,.2f" : fmt === "percent" ? ",.1f" : ",.0f"
                return (
                  <Metric
                    name={`${agg === 'AVG' ? 'Avg' : 'Total'} ${titleCase(col.name)}`}
                    col={3}
                    sql={`SELECT ${agg}("${col.name}") as value FROM "${table}"`}
                    value={{ field: "value", type: "number", format: spec }}
                  />
                )
              })}
            </Row>

            {/* Time-series chart if the table has a date column */}
            {timeCol && kpiCols.length > 0 && (
              <Row>
                <Chart
                  name={`${titleCase(table)} Over Time`}
                  chart="area"
                  col={cats.length > 0 ? 8 : 12}
                  sql={`
                    SELECT
                      STRFTIME(DATE_TRUNC('month', "${timeCol.name}"), '%Y-%m') AS month
                      ${kpiCols.slice(0, 4).map(c => `, ${bestAggregate(c.name)}("${c.name}") AS "${titleCase(c.name)}"`).join('')}
                    FROM "${table}"
                    GROUP BY 1 ORDER BY 1
                  `}
                  x="month"
                  y={kpiCols.slice(0, 4).map(c => titleCase(c.name))}
                />
                {cats.length > 0 && (
                  <Chart
                    name={`By ${titleCase(cats[0].name)}`}
                    chart="pie"
                    col={4}
                    sql={`
                      SELECT "${cats[0].name}" as label, COUNT(*) as value
                      FROM "${table}"
                      GROUP BY 1 ORDER BY 2 DESC
                    `}
                    label="label"
                    value="value"
                  />
                )}
              </Row>
            )}

            {/* Categorical breakdowns */}
            {cats.length > 0 && kpiCols.length > 0 && (
              <Row>
                <Chart
                  name={`${titleCase(kpiCols[0].name)} by ${titleCase(cats[0].name)}`}
                  chart="bar"
                  col={cats.length >= 2 ? 6 : 12}
                  sql={`
                    SELECT "${cats[0].name}", ${bestAggregate(kpiCols[0].name)}("${kpiCols[0].name}") as "${titleCase(kpiCols[0].name)}"
                    FROM "${table}"
                    GROUP BY 1 ORDER BY 2 DESC LIMIT 15
                  `}
                  x={cats[0].name}
                  y={[titleCase(kpiCols[0].name)]}
                />
                {cats.length >= 2 && (
                  <Chart
                    name={`${titleCase(kpiCols[0].name)} by ${titleCase(cats[1].name)}`}
                    chart="bar"
                    col={6}
                    sql={`
                      SELECT "${cats[1].name}", ${bestAggregate(kpiCols[0].name)}("${kpiCols[0].name}") as "${titleCase(kpiCols[0].name)}"
                      FROM "${table}"
                      GROUP BY 1 ORDER BY 2 DESC LIMIT 15
                    `}
                    x={cats[1].name}
                    y={[titleCase(kpiCols[0].name)]}
                  />
                )}
              </Row>
            )}

            {/* Data preview */}
            <Row>
              <Table
                name={`${titleCase(table)} — Recent Data`}
                col={12}
                sql={`SELECT ${displayCols.map(c => `"${c.name}"`).join(', ')} FROM "${table}" ORDER BY ${orderCol} LIMIT 10`}
              />
            </Row>
          </Tab>
        )
      })}
    </Tabs>
  </Dashboard>
)
