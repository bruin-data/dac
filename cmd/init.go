package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bruin-data/dac/pkg/telemetry"
	analytics "github.com/rudderlabs/analytics-go/v4"
	"github.com/urfave/cli/v3"
)

type initFile struct {
	path          string
	content       []byte
	symlinkTarget string
}

//go:embed init_demo.duckdb
var initDuckDB []byte

func initCmd() *cli.Command {
	return &cli.Command{
		Name:      "init",
		Usage:     "Create a new DAC project",
		ArgsUsage: "[path]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "template",
				Aliases: []string{"t"},
				Usage:   "Project template: starter, sql, semantic, tsx",
				Value:   "starter",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite scaffold files if they already exist",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			target := "."
			if cmd.Args().Len() > 0 {
				target = cmd.Args().First()
			}
			return runInit(target, cmd.String("template"), cmd.Bool("force"))
		},
	}
}

func runInit(target, template string, force bool) error {
	template = normalizeInitTemplate(template)
	telemetry.SetTemplateName(template)
	telemetry.SendEvent("template_selected", analytics.Properties{
		"template": template,
	})
	files, err := initTemplateFiles(template)
	if err != nil {
		return err
	}

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if info, err := os.Stat(targetAbs); err == nil && !info.IsDir() {
		return fmt.Errorf("init target exists and is not a directory: %s", target)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(targetAbs, 0o755); err != nil {
		return fmt.Errorf("creating project directory: %w", err)
	}

	var conflicts []string
	for _, file := range files {
		path := filepath.Join(targetAbs, filepath.FromSlash(file.path))
		if _, err := os.Lstat(path); err == nil {
			conflicts = append(conflicts, file.path)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if len(conflicts) > 0 && !force {
		return fmt.Errorf("target contains files that would be overwritten: %s (use --force to overwrite)", strings.Join(conflicts, ", "))
	}

	for _, file := range files {
		path := filepath.Join(targetAbs, filepath.FromSlash(file.path))
		if force {
			if _, err := os.Lstat(path); err == nil {
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf("removing existing %s: %w", file.path, err)
				}
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", file.path, err)
		}
		if file.symlinkTarget != "" {
			if err := os.Symlink(file.symlinkTarget, path); err != nil {
				return fmt.Errorf("linking %s: %w", file.path, err)
			}
			continue
		}
		if err := os.WriteFile(path, file.content, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", file.path, err)
		}
	}
	if err := initGitRepository(targetAbs); err != nil {
		return err
	}

	fmt.Printf("Created DAC project in %s\n", targetAbs)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  cd %s\n", shellPath(target))
	fmt.Println("  dac validate --dir .")
	fmt.Println("  dac serve --dir . --open")
	fmt.Println()
	fmt.Println("If you want to execute live queries, make sure the Bruin CLI is installed and on your PATH.")

	return nil
}

func initGitRepository(targetAbs string) error {
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = targetAbs
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("git is required to initialize DAC projects because Bruin uses the Git repository root for project discovery. Install git, then run `git init` in %s", targetAbs)
		}
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf("initializing git repository: %w: %s", err, msg)
		}
		return fmt.Errorf("initializing git repository: %w", err)
	}
	return nil
}

func normalizeInitTemplate(template string) string {
	switch strings.ToLower(strings.TrimSpace(template)) {
	case "", "starter":
		return "starter"
	case "sql", "basic", "basic-yaml", "yaml":
		return "sql"
	case "semantic", "semantic-yaml", "semantic-yml":
		return "semantic"
	case "tsx", "semantic-tsx":
		return "tsx"
	default:
		return strings.ToLower(strings.TrimSpace(template))
	}
}

func initTemplateFiles(template string) ([]initFile, error) {
	base := []initFile{
		initTextFile(".bruin.yml", initBruinConfig),
		initTextFile("README.md", initProjectReadme(template)),
		{path: "data/dac-demo.duckdb", content: initDuckDB},
	}
	skillFiles, err := initSkillFiles()
	if err != nil {
		return nil, err
	}
	base = append(base, skillFiles...)

	switch template {
	case "starter":
		return append(base,
			initTextFile("dashboards/sales.yml", initSQLDashboard),
			initTextFile("dashboards/semantic-sales.yml", initSemanticDashboard),
			initTextFile("semantic/sales.yml", initSemanticModel),
		), nil
	case "sql":
		return append(base,
			initTextFile("dashboards/sales.yml", initSQLDashboard),
		), nil
	case "semantic":
		return append(base,
			initTextFile("dashboards/semantic-sales.yml", initSemanticDashboard),
			initTextFile("semantic/sales.yml", initSemanticModel),
		), nil
	case "tsx":
		return append(base,
			initTextFile("dashboards/semantic-sales.dashboard.tsx", initSemanticTSXDashboard),
			initTextFile("semantic/sales.yml", initSemanticModel),
		), nil
	default:
		return nil, fmt.Errorf("unknown init template %q (expected starter, sql, semantic, or tsx)", template)
	}
}

func initTextFile(path, content string) initFile {
	return initFile{path: path, content: []byte(content)}
}

func initSymlink(path, target string) initFile {
	return initFile{path: path, symlinkTarget: target}
}

func initSkillFiles() ([]initFile, error) {
	files := make([]initFile, 0, len(dacSkills)*2)
	for _, skill := range dacSkills {
		files = append(files, initTextFile(skill.ClaudePath, skill.Content))

		target, err := filepath.Rel(
			filepath.Dir(filepath.FromSlash(skill.CodexPath)),
			filepath.Dir(filepath.FromSlash(skill.ClaudePath)),
		)
		if err != nil {
			return nil, fmt.Errorf("resolving Codex skill symlink for %s: %w", skill.Name, err)
		}
		files = append(files, initSymlink(skill.CodexPath, filepath.ToSlash(target)))
	}
	return files, nil
}

func shellPath(path string) string {
	if path == "." || path == "" {
		return "."
	}
	return path
}

func indentBlock(s string, spaces int) string {
	prefix := strings.Repeat(" ", spaces)
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

const initBruinConfig = `default_environment: default

environments:
  default:
    connections:
      duckdb:
        - name: local_duckdb
          path: data/dac-demo.duckdb
          read_only: true
`

func initProjectReadme(template string) string {
	queryCommand := `dac query --dir . --dashboard "Semantic Sales" --widget "Revenue"`
	if template == "sql" {
		queryCommand = `dac query --dir . --dashboard "Sales Overview" --widget "Total Revenue"`
	}

	return `# DAC Project

This project was generated with ` + "`dac init`" + `.

` + "`dac init`" + ` initialized this directory as a Git repository so Bruin can discover the project root immediately.

## Commands

` + "```shell" + `
dac validate --dir .
dac serve --dir . --open
` + "```" + `

The generated dashboards use a local DuckDB connection named ` + "`local_duckdb`" + `. The starter queries include inline sample data, so there is no seed step.

## Agent Skills

This project includes DAC's bundled dashboard authoring skill:

- ` + "`.claude/skills/create-dashboard/SKILL.md`" + `
- ` + "`.codex/skills/create-dashboard`" + ` symlinked to the same skill for Codex

Restart your agent session to pick up newly installed skills.

To inspect one generated widget from the command line:

` + "```shell" + `
` + queryCommand + `
` + "```" + `
`
}

const initSalesCTE = `WITH sales AS (
  SELECT * FROM (VALUES
    ('2024-01-15', 'North America', 'online', 1200.00, 1001, 501),
    ('2024-01-27', 'North America', 'retail', 860.00, 1002, 502),
    ('2024-02-08', 'Europe', 'online', 1430.00, 1003, 503),
    ('2024-02-22', 'APAC', 'partner', 980.00, 1004, 504),
    ('2024-03-12', 'Europe', 'retail', 760.00, 1005, 505),
    ('2024-03-28', 'North America', 'online', 1890.00, 1006, 501),
    ('2024-04-09', 'APAC', 'online', 1320.00, 1007, 506),
    ('2024-04-21', 'Europe', 'partner', 1110.00, 1008, 503)
  ) AS t(created_at, region, channel, amount, order_id, customer_id)
)`

var initSQLDashboard = strings.NewReplacer(
	"__INIT_CTE_QUERY__", indentBlock(initSalesCTE, 6),
	"__INIT_CTE_WIDGET__", indentBlock(initSalesCTE, 10),
).Replace(`name: Sales Overview
description: SQL-backed starter dashboard generated by dac init
connection: local_duckdb

filters:
  - name: region
    type: select
    default: All
    options:
      values: [All, North America, Europe, APAC]
  - name: date_range
    type: date-range
    default: all_time

queries:
  filtered_sales:
    sql: |
__INIT_CTE_QUERY__
      SELECT *
      FROM sales
      WHERE created_at >= '{{ filters.date_range.start }}'
        AND created_at <= '{{ filters.date_range.end }}'
      {% if filters.region != 'All' %}
        AND region = '{{ filters.region }}'
      {% endif %}

rows:
  - widgets:
      - name: Total Revenue
        type: metric
        value:
          field: value
          type: number
          format: "$,.0f"
        col: 3
        sql: |
__INIT_CTE_WIDGET__
          SELECT SUM(amount) AS value
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}

      - name: Sales Count
        type: metric
        col: 3
        sql: |
__INIT_CTE_WIDGET__
          SELECT COUNT(*) AS value
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}
        value:
          field: value
          type: number
          format: ",.0f"

      - name: Unique Customers
        type: metric
        col: 3
        sql: |
__INIT_CTE_WIDGET__
          SELECT COUNT(DISTINCT customer_id) AS value
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}
        value:
          field: value
          type: number
          format: ",.0f"

      - name: Average Sale Value
        type: metric
        col: 3
        sql: |
__INIT_CTE_WIDGET__
          SELECT ROUND(AVG(amount), 2) AS value
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}
        value:
          field: value
          type: number
          format: "$,.2f"

  - widgets:
      - name: Revenue Trend
        type: chart
        chart: area
        col: 8
        sql: |
__INIT_CTE_WIDGET__
          SELECT substr(created_at, 1, 7) AS month, SUM(amount) AS revenue
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          {% if filters.region != 'All' %}
            AND region = '{{ filters.region }}'
          {% endif %}
          GROUP BY 1
          ORDER BY 1
        x: month
        y: [revenue]

      - name: Revenue by Region
        type: chart
        chart: bar
        col: 4
        sql: |
__INIT_CTE_WIDGET__
          SELECT region, SUM(amount) AS revenue
          FROM sales
          WHERE created_at >= '{{ filters.date_range.start }}'
            AND created_at <= '{{ filters.date_range.end }}'
          GROUP BY 1
          ORDER BY 2 DESC
        x: region
        y: [revenue]

  - widgets:
      - name: Recent Sales
        type: table
        col: 12
        query: filtered_sales
        columns:
          - name: created_at
            label: Date
          - name: region
            label: Region
          - name: channel
            label: Channel
          - name: amount
            label: Amount
            format: currency
`)

const initSemanticModel = `name: sales
label: Sales
description: Semantic model over inline sample sales data
source:
  table: |
    (SELECT * FROM (VALUES
      ('2024-01-15', 'North America', 'online', 1200.00, 1001, 501),
      ('2024-01-27', 'North America', 'retail', 860.00, 1002, 502),
      ('2024-02-08', 'Europe', 'online', 1430.00, 1003, 503),
      ('2024-02-22', 'APAC', 'partner', 980.00, 1004, 504),
      ('2024-03-12', 'Europe', 'retail', 760.00, 1005, 505),
      ('2024-03-28', 'North America', 'online', 1890.00, 1006, 501),
      ('2024-04-09', 'APAC', 'online', 1320.00, 1007, 506),
      ('2024-04-21', 'Europe', 'partner', 1110.00, 1008, 503)
    ) AS t(created_at, region, channel, amount, order_id, customer_id))

dimensions:
  - name: created_at
    type: time
    granularities:
      month: substr(created_at, 1, 7)
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
  - name: unique_customers
    expression: count(distinct customer_id)
  - name: avg_sale_value
    expression: "{revenue} / {sales_count}"
  - name: online_revenue
    expression: sum(amount)
    filter: "channel = 'online'"

segments:
  - name: online
    filter: "channel = 'online'"
`

const initSemanticDashboard = `name: Semantic Sales
description: Semantic-layer starter dashboard generated by dac init
connection: local_duckdb
model: sales

filters:
  - name: region
    type: select
    default: North America
    options:
      values: [North America, Europe, APAC]
  - name: date_range
    type: date-range
    default: all_time

queries:
  online_by_region:
    dimensions:
      - name: region
    metrics: [revenue]
    segments: [online]
    sort:
      - name: revenue
        direction: desc

rows:
  - widgets:
      - name: Revenue
        type: metric
        metric: revenue
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
        value:
          field: revenue
          type: number
          format: "$,.0f"
        col: 3

      - name: Sales Count
        type: metric
        metric: sales_count
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
        value:
          field: sales_count
          type: number
          format: ",.0f"
        col: 3

      - name: Unique Customers
        type: metric
        metric: unique_customers
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
        value:
          field: unique_customers
          type: number
          format: ",.0f"
        col: 3

      - name: Average Sale Value
        type: metric
        metric: avg_sale_value
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
        value:
          field: avg_sale_value
          type: number
          format: "$,.2f"
        col: 3

  - widgets:
      - name: Revenue Trend
        type: chart
        chart: area
        dimension: created_at
        granularity: month
        metrics: [revenue]
        filters:
          - dimension: region
            operator: equals
            value: "{{ filters.region }}"
          - dimension: created_at
            operator: between
            value:
              start: "{{ filters.date_range.start }}"
              end: "{{ filters.date_range.end }}"
        sort:
          - name: created_at
            direction: asc
        col: 8

      - name: Online Revenue by Region
        type: chart
        chart: bar
        query: online_by_region
        col: 4

  - widgets:
      - name: Sales Breakdown
        type: table
        dimensions:
          - name: region
          - name: channel
        metrics: [revenue, sales_count]
        sort:
          - name: revenue
            direction: desc
        columns:
          - name: region
            label: Region
          - name: channel
            label: Channel
          - name: revenue
            label: Revenue
            format: currency
          - name: sales_count
            label: Sales
            format: number
        col: 12
`

const initSemanticTSXDashboard = `export default (
  <Dashboard
    name="Semantic Sales"
    description="Semantic-layer TSX starter dashboard generated by dac init"
    connection="local_duckdb"
    model="sales"
  >
    <Filter
      name="region"
      type="select"
      default="North America"
      options={{ values: ["North America", "Europe", "APAC"] }}
    />
    <Filter name="date_range" type="date-range" default="all_time" />

    <Query
      name="onlineByRegion"
      dimensions={[{ name: "region" }]}
      metrics={["revenue"]}
      segments={["online"]}
      sort={[{ name: "revenue", direction: "desc" }]}
    />

    <Row>
      <Metric
        name="Revenue"
        metric="revenue"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        value={{ field: "revenue", type: "number", format: "$,.0f" }}
        col={3}
      />
      <Metric
        name="Sales Count"
        metric="sales_count"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        value={{ field: "sales_count", type: "number", format: ",.0f" }}
        col={3}
      />
      <Metric
        name="Unique Customers"
        metric="unique_customers"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        value={{ field: "unique_customers", type: "number", format: ",.0f" }}
        col={3}
      />
      <Metric
        name="Average Sale Value"
        metric="avg_sale_value"
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        value={{ field: "avg_sale_value", type: "number", format: "$,.2f" }}
        col={3}
      />
    </Row>

    <Row>
      <Chart
        name="Revenue Trend"
        chart="area"
        dimension="created_at"
        granularity="month"
        metrics={["revenue"]}
        filters={[
          { dimension: "region", operator: "equals", value: "{{ filters.region }}" },
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        sort={[{ name: "created_at", direction: "asc" }]}
        col={8}
      />
      <Chart
        name="Online Revenue by Region"
        chart="bar"
        query="onlineByRegion"
        col={4}
      />
    </Row>

    <Row>
      <Table
        name="Sales Breakdown"
        dimensions={[{ name: "region" }, { name: "channel" }]}
        metrics={["revenue", "sales_count"]}
        sort={[{ name: "revenue", direction: "desc" }]}
        columns={[
          { name: "region", label: "Region" },
          { name: "channel", label: "Channel" },
          { name: "revenue", label: "Revenue", format: "currency" },
          { name: "sales_count", label: "Sales", format: "number" },
        ]}
        col={12}
      />
    </Row>
  </Dashboard>
)
`
