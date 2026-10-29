# Repository CI

DAC's `CI` workflow runs on pull requests targeting any branch, including stacked
PRs targeting another feature branch, and on pushes to `main`.

`Docs Build` also accepts pull requests targeting any branch, but only when files
under `docs/` change. Its push trigger remains limited to `main` and `docs/` changes.

PR checks do not trigger releases or documentation deployments. Those workflows
retain their separate push and manual-dispatch triggers.
