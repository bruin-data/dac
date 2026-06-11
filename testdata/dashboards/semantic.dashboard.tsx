export default (
  <Dashboard
    name="Semantic Sales TSX Example"
    description="TSX dashboard using the external sales semantic model"
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
        model="sales"
        dimensions={[{ name: "region" }, { name: "channel" }]}
        metrics={["revenue", "sales_count"]}
        filters={[
          { dimension: "created_at", operator: "between", value: { start: "{{ filters.date_range.start }}", end: "{{ filters.date_range.end }}" } },
        ]}
        sort={[{ name: "revenue", direction: "desc" }]}
        limit={20}
        col={12}
      />
    </Row>
  </Dashboard>
)
