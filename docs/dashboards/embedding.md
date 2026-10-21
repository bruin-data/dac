# Embedding

The widgets, filter bar and layout that `dac serve` renders are also published as a React package, `@bruin-data/dac-ui`. Use it to render dac dashboards inside another React app that fetches dashboards and query results its own way.

## Install

Each release attaches the package as `dac-ui.tgz`:

```shell
npm install https://github.com/bruin-data/dac/releases/download/<tag>/dac-ui.tgz
```

`<tag>` is a release tag such as `v0.29.0`. To update, change the tag and reinstall.

The host provides `react`, `react-dom` (19) and `@tanstack/react-query` (5).

## Styles

The components use Tailwind CSS v4 classes, so they are styled by the host's Tailwind build. Import the stylesheet and add the package as a source:

```css
@import "tailwindcss";
@import "@bruin-data/dac-ui/styles.css";
@source "../node_modules/@bruin-data/dac-ui/dist-lib";
```

`styles.css` includes the Geist font files, so nothing is loaded from a CDN.

## Data client

Widgets load data through a `DataClient`. Without a provider they call a dac server at `/api/v1` on the page's origin; pass your own to load from anywhere else:

```tsx
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DataClientProvider, type DataClient } from "@bruin-data/dac-ui";

const client: DataClient = {
  listDashboards: async () => [...],
  getDashboard: async (name) => {...},
  fetchWidgetData: async (dashboardName, widgetId, filters) => ({ columns: [{ name: "revenue" }], rows: [[1200]] }),
  fetchFilterOptions: async (dashboardName, filterName, filters) => ["EU", "US"],
};

<QueryClientProvider client={new QueryClient()}>
  <DataClientProvider client={client}>{/* dashboards */}</DataClientProvider>
</QueryClientProvider>
```

Widget ids are `r{row}-w{widget}`, zero-based positions in the dashboard's `rows`. A tab of a `type: tabs` widget is `r{row}-w{widget}::{tab name}`.

## Rendering a dashboard

Compose the layout from the exported pieces. This example leaves out row tabs (`rows[].tab`); see `DashboardView.tsx` for the full layout.

```tsx
import { useState } from "react";
import {
  BruinFilterBar, BruinRow, BruinWidgetContainer, BruinWidgetFrame, DataWidget, TabbedWidget,
  TemplateProvider, bruinDarkTemplate, buildDefaultFilters, useDashboard, useFilterOptions,
} from "@bruin-data/dac-ui";

function Dashboard({ name }: { name: string }) {
  const { data: dashboard } = useDashboard(name);
  const [filters, setFilters] = useState<Record<string, unknown> | null>(null);
  const values = filters ?? (dashboard ? buildDefaultFilters(dashboard) : {});
  const filterDefs = useFilterOptions(dashboard, values);
  if (!dashboard) return null;

  return (
    <TemplateProvider template={bruinDarkTemplate} className="h-full">
      <BruinFilterBar filters={filterDefs} values={values} onChange={(n, v) => setFilters({ ...values, [n]: v })} />
      {dashboard.rows.map((row, i) => (
        <BruinRow key={i} height={row.height}>
          {row.widgets.map((widget, j) => {
            const id = `r${i}-w${j}`;
            const props = { dashboardName: name, widget, filters: values, WidgetFrame: BruinWidgetFrame };
            return (
              <BruinWidgetContainer key={id} col={widget.col ?? Math.floor(12 / row.widgets.length)}>
                {widget.tabs?.length ? <TabbedWidget {...props} baseId={id} /> : <DataWidget {...props} widgetId={id} />}
              </BruinWidgetContainer>
            );
          })}
        </BruinRow>
      ))}
    </TemplateProvider>
  );
}
```

`TemplateProvider` takes the same templates and tokens as [themes](./themes.md). Its `className` replaces the default `min-h-screen` sizing.

The individual widgets (`ChartWidget`, `TableWidget`, `MetricWidget`, `TextWidget`) take `{ widget, data }` and can be rendered on their own.

## Content Security Policy

The components render under a strict policy (`script-src 'self'`, no `'unsafe-eval'`): Vega-Lite charts evaluate expressions with Vega's interpreter instead of generating code. `image` widgets and image cells load their `https:` URLs, so allow those in `img-src` if you use them.
