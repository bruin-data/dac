# Connections

DAC uses the same `.bruin.yml` configuration file as the Bruin CLI for database connections.

## Configuration File

DAC auto-discovers `.bruin.yml` by searching upward from the dashboard directory. You can also specify it explicitly:

```shell
dac serve --config /path/to/.bruin.yml
```

## Structure

```yaml
default_environment: default

environments:
  default:
    connections:
      duckdb:
        - name: my_db
          path: ./data.duckdb

      postgres:
        - name: analytics
          host: localhost
          port: 5432
          database: analytics
          user: admin
          password: secret
          ssl_mode: disable

      bigquery:
        - name: warehouse
          project_id: my-gcp-project
          dataset_id: analytics
```

## Supported Databases

DAC supports any database connection that Bruin supports:

| Type | Connection Fields |
|------|-------------------|
| `duckdb` | `name`, `path` |
| `postgres` | `name`, `host`, `port`, `database`, `user`, `password`, `ssl_mode` |
| `bigquery` | `name`, `project_id`, `dataset_id`, `credentials_path` |
| `snowflake` | `name`, `account`, `user`, `password`, `database`, `schema`, `warehouse`, `role` |
| `mysql` | `name`, `host`, `port`, `database`, `user`, `password` |
| `mssql` | `name`, `host`, `port`, `database`, `user`, `password` |
| `redshift` | `name`, `host`, `port`, `database`, `user`, `password` |
| `clickhouse` | `name`, `host`, `port`, `database`, `user`, `password` |
| `databricks` | `name`, `host`, `token`, `path`, `catalog`, `schema` |
| `athena` | `name`, `region`, `database`, `output_location`, `access_key`, `secret_key` |

See the [Bruin CLI documentation](https://getbruin.com/docs/bruin/) for the full list.

## Environments

Multiple environments let you switch between dev/staging/prod databases:

Without `--environment`, DAC uses `default_environment`, or an environment named `default` when that setting is omitted.

```yaml
default_environment: dev

environments:
  dev:
    connections:
      duckdb:
        - name: my_db
          path: ./dev.duckdb
  prod:
    connections:
      postgres:
        - name: my_db
          host: prod-db.example.com
          port: 5432
          database: analytics
          user: readonly
          password: ${DB_PASSWORD}
```

Switch environments:

```shell
dac serve --environment prod
```

## Testing Connections

Verify all connections are reachable:

```shell
dac connections
```

See [`dac connections`](/commands/connections) for details.

## Connection Override

Dashboards set a default connection, and individual widgets or queries can override it:

```yaml
# Dashboard level
connection: my_db

# Query level override
queries:
  warehouse_data:
    sql: SELECT * FROM summary
    connection: warehouse

# Widget level override
rows:
  - widgets:
      - name: From Analytics
        type: table
        sql: SELECT * FROM events
        connection: analytics_db
```
