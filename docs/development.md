# Repository CI

DAC's `CI` workflow runs on pull requests targeting any branch, including stacked
PRs targeting another feature branch, and on pushes to `main`.

`Docs Build` also accepts pull requests targeting any branch, but only when files
under `docs/` change. Its push trigger remains limited to `main` and `docs/` changes.

PR checks do not trigger releases or documentation deployments. Those workflows
retain their separate push and manual-dispatch triggers.

## Launching dac from bruin

`bruin dac <args>` runs the standalone `dac` binary as a child process. bruin
sets two environment variables so dac uses that bruin and presents itself as
`bruin dac`. dac reads both once at startup, in `main`, and passes them down
through `cmd.BuildInfo`. With neither set, dac behaves exactly as standalone
`dac`.

| Variable | Effect |
|---|---|
| `DAC_BRUIN_PATH` | Path to the bruin binary every query runs through: `query`, `check`, `validate --with-database`, `connections`, `serve`, `build`, and `export slides`. bruin sets it to its own executable so queries never reach another `bruin` on `PATH`. Unset means `bruin` on `PATH`. `init` drops its "make sure the Bruin CLI is on your PATH" hint when it's set. |
| `DAC_HOST=bruin` | Hides the `upgrade`/`update` and `version` commands and the `--version` flag, and turns off the update check and its nudge, since bruin pins and installs dac. Help, usage, `init`'s next steps, and other messages that tell the user what to run say `bruin dac` instead of `dac`. The stale-skill notice from `serve`/`validate` points at `bruin dac skills update --dir <project root>` instead of `dac upgrade`. Any other value is treated as unset. |

The two are independent. bruin sets both:

```shell
DAC_BRUIN_PATH=/path/to/bruin DAC_HOST=bruin ~/.bruin/dac/<version>/dac serve --dir .
```

Telemetry opt-out still uses `TELEMETRY_OPTOUT` or `DO_NOT_TRACK`; `DAC_HOST`
doesn't change it.
