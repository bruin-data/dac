# Contributing to DAC

Thanks for contributing.

DAC is a Dashboard-as-Code tool with three main surfaces:

- the Go CLI and backend
- the embedded React frontend
- the dashboard authoring model across YAML, TSX, and the semantic layer

Good contributions usually improve one of those areas while keeping the other two coherent.

## Before You Start

- Open an issue first for larger changes, schema changes, or anything that affects the public authoring model.
- Keep pull requests focused. Small, reviewable changes move faster than broad refactors.
- If you change user-facing behavior, update docs and examples in the same pull request.

## Local Setup

DAC uses the Bruin CLI for query execution, so install `bruin` if you want to run live queries locally.

Prerequisites:

- Go 1.25+
- Node.js 20+
- `make`
- Bruin CLI on your `PATH` for `dac serve`, `dac query`, and `dac check`

Install dependencies:

```bash
make deps
```

Run the core verification loop:

```bash
make test
make build
```

Run DAC against an example project:

```bash
./bin/dac serve --dir examples/basic-yaml
./bin/dac validate --dir examples/semantic-yaml
```

## Development Guidelines

- Use `make` commands instead of ad-hoc `go build`, `go test`, or `npm run build`.
- Add or update tests for any behavioral change.
- After every change, scan `docs/` for areas the change touches and update them in the same PR — add a new doc page if nothing existing fits.
- Update one of the curated projects in `examples/` if the authoring model changes.
- Keep YAML and TSX behavior aligned when a feature exists in both surfaces.

## Repository Map

- `cmd/`: CLI commands and flag handling
- `pkg/dashboard/`: dashboard loading, validation, YAML/TSX parsing
- `pkg/semantic/`: semantic model loading and SQL compilation
- `pkg/server/`: HTTP API and runtime query orchestration
- `pkg/query/`: query execution backends
- `frontend/`: embedded React frontend
- `examples/`: runnable public sample projects
- `docs/`: VitePress documentation source
- `testdata/`: internal fixtures used by unit tests

## Pull Requests

Before opening a PR, run:

```bash
make test
make build
```

If your change affects examples or runtime behavior, also run the relevant example manually:

```bash
./bin/dac validate --dir examples/basic-yaml
./bin/dac validate --dir examples/semantic-yaml
```

PRs should include:

- a clear problem statement
- the implementation approach
- any tradeoffs or intentional limitations
- screenshots or short recordings for visible UI changes

## Reporting Problems

- Bugs and feature requests: GitHub issues
- Security reports: `security@getbruin.com`
- General support questions: `support@getbruin.com`
