export default (
  <Dashboard
    name="Semantic TSX"
    description="External semantic-model dashboard from TSX"
    connection="warehouse"
    model="sales"
    models={{ sales_model: "sales" }}
  >
    <Filter
      name="country"
      type="select"
      default="US"
      options={{ values: ["US", "CA"] }}
    />

    <Query
      name="completedByCountry"
      model="sales_model"
      dimensions={[{ name: "country" }]}
      metrics={["revenue"]}
      segments={["completed"]}
      sort={[{ name: "revenue", direction: "desc" }]}
      limit={5}
    />

    <Row>
      <Metric name="Revenue" metric="revenue" col={4} />
      <Metric
        name="Average Order Value"
        metric="avg_order_value"
        filters={[{ dimension: "country", operator: "equals", value: "{{ filters.country }}" }]}
        col={4}
      />
      <Metric name="Completed Revenue" metric="completed_revenue" col={4} />
    </Row>

    <Row>
      <Chart
        name="Revenue Trend"
        chart="line"
        dimension="order_date"
        granularity="month"
        metrics={["revenue"]}
        filters={[{ dimension: "country", operator: "equals", value: "{{ filters.country }}" }]}
        sort={[{ name: "order_date", direction: "asc" }]}
        col={8}
      />
      <Chart
        name="Completed By Country"
        chart="bar"
        query="completedByCountry"
        col={4}
      />
    </Row>

    <Row>
      <Table
        name="Country Table"
        model="sales_model"
        dimensions={[{ name: "country" }]}
        metrics={["revenue", "order_count"]}
        filters={[{ dimension: "country", operator: "equals", value: "{{ filters.country }}" }]}
        sort={[{ name: "revenue", direction: "desc" }]}
        limit={10}
        col={12}
      />
    </Row>
  </Dashboard>
)
