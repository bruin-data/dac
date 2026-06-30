# Schemas

DAC YAML files are validated against versioned Bruin schema identifiers. The `schema` field is optional; when it is omitted, DAC assumes the current v1 schema for that file type.

## Schema IDs

| File type | Schema |
|-----------|--------|
| Dashboard YAML | `https://getbruin.com/schemas/dac/dashboard/v1` |
| Semantic model YAML | `v1` (the legacy `https://getbruin.com/schemas/dac/semantic-model/v1` id is still accepted) |
| Theme YAML | `https://getbruin.com/schemas/dac/theme/v1` |

Dashboard example:

```yaml
name: Sales
rows:
  - widgets:
      - name: Revenue
        type: metric
        sql: SELECT SUM(amount) AS value FROM sales
        value: { field: value }
```

Semantic model example:

```yaml
name: sales
source:
  table: sales
metrics:
  - name: revenue
    expression: sum(amount)
```

Theme example:

```yaml
name: corporate
tokens:
  background: "#FFFFFF"
```

Add `schema` only when you want to pin an explicit contract version:

```yaml
schema: https://getbruin.com/schemas/dac/dashboard/v1
name: Sales
rows:
  - widgets:
      - name: Notes
        type: text
        content: v1 schema is pinned explicitly
```

## Validation

`dac validate`, `dac check`, `dac serve`, and `dac build` validate YAML files against the v1 schema for that file type. If `schema` is present, it must match the v1 schema ID.

JSON Schema validates structure: required fields, field types, enum values, and unknown fields. DAC's Go validators still validate meaning: query references, semantic model references, metric references, segments, filters, sort fields, and layout rules.

## Extension Fields

Schema-defined objects allow extension fields that start with `x-`:

```yaml
name: Sales
x-owner: analytics
rows:
  - widgets:
      - name: Notes
        type: text
        content: Owned by analytics
```

Use `x-` fields for local metadata. DAC ignores them.
