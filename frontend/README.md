# DAC Frontend

This is the React frontend embedded into the DAC Go binary.

## Development

Run from the repository root:

```bash
make dev-frontend
```

The Vite dev server proxies API calls to the Go backend on port `8321`.

For a production build, use the root Makefile:

```bash
make frontend
make build
```

`make frontend` writes assets to `frontend/dist`, and the Go binary embeds that directory through `embed.go`.

## Component Library

The same components are packaged as `@bruin-data/dac-ui` for other React apps (see `docs/dashboards/embedding.md`):

```bash
make frontend-lib
```

The entry point is `src/lib.ts`, the output goes to `frontend/dist-lib`, and component styles live in `src/styles/dac.css`. Releases attach the package as `dac-ui.tgz`.

## Useful Commands

From `frontend/`:

```bash
npm run dev
npm run build
npm run build:lib
npm run typecheck
npm run lint
```

Prefer the root `make` targets for normal development and verification so frontend embedding stays consistent with the backend build.
