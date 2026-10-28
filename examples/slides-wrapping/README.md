# Slides wrapping demo

Inline sample data; no database, connection configuration, or Bruin query execution is needed. Run from the repository root using a binary built from this branch:

```sh
make build
./bin/dac validate --dir examples/slides-wrapping/dashboards
./bin/dac serve --dir examples/slides-wrapping/dashboards
```

To export, use your [Google credentials](../../docs/commands/export.md#authentication):

```sh
./bin/dac export slides \
  --dir examples/slides-wrapping/dashboards \
  --dashboard "Slides wrapping demo"
```

Expected deck: **7 slides**, all widgets inside the slide margins.

| Slide | Content |
| --- | --- |
| 1 | Dashboard title |
| 2 | Revenue: $12,400 (`col: 8`) |
| 3 | Orders: 318 (`col: 8`, wrapped) |
| 4 | Bookings trend chart (`col: 8`) |
| 5 | Route summary table (`col: 8`) + hotel occupancy: 78% (`col: 4`) |
| 6 | Travel metrics: $24,800, from the first widget tab |
| 7 | Repeat bookings: 84 (`col: 8`, wrapped) |

The first row reproduces the reported overflow case: two `col: 8` widgets. The second checks continuation slides with mixed widget types; the third checks that wrapped tabbed widgets keep their correct data.
