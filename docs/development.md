# Repository CI

DAC's `CI` workflow runs on pull requests targeting any branch, including stacked
PRs targeting another feature branch, and on pushes to `main`.

`Docs Build` also accepts pull requests targeting any branch, but only when files
under `docs/` change. Its push trigger remains limited to `main` and `docs/` changes.

PR checks do not trigger releases or documentation deployments. Those workflows
retain their separate push and manual-dispatch triggers.

## Release Assets

Tagged releases (`release.yml`) and edge releases (`edge-release.yml`) publish the
goreleaser archives and `checksums.txt`, plus these frontend assets:

| Asset | Built with | Used by |
|---|---|---|
| `dac-frontend-standalone.tar.gz` | `base: "/"`, packed from the `frontend/dist` the release binaries embed | Binaries that embed dac, such as `bruin dac` |
| `dac-frontend-standalone.tar.gz.sha256` | `sha256sum` output (`<hash>  dac-frontend-standalone.tar.gz`) | Checksum verification for the asset above |
| `dac-frontend.tar.gz` | `base: "./"` (`DAC_EMBED=1`), tagged releases only | The Bruin VS Code extension webview |

The standalone tarball has `index.html` and `assets/` at its root. Its asset URLs are
absolute, so deep links such as `/dashboards/<name>` load correctly from `dac serve`.
Don't use `dac-frontend.tar.gz` for a server: its relative URLs break deep links.

## Embedding dac in Another Binary

Another Go binary can run dac in-process through `github.com/bruin-data/dac/cmd`:

```go
err := dacmd.Run(ctx, append([]string{"bruin dac"}, args...), frontendFS, dacmd.BuildInfo{
	Version:   dacVersion,
	BruinPath: hostExecutable,
	Embedded:  true,
})
```

`BuildInfo` fields for embedders:

- `BruinPath`: the bruin binary dac runs queries through, for `serve`, `build`,
  `check`, `query`, `validate`, `connections`, and `export slides`. Empty means
  `bruin` on `PATH`, which is what standalone dac uses.
- `Embedded`: leaves out the `upgrade`/`update` and `version` commands and the
  `--version` flag, which the host binary covers. It also takes the root command
  name from `args[0]`, so help reads `bruin dac serve` rather than `dac serve`.
  Standalone dac always shows `dac`, whatever `os.Args[0]` is.

The update-check nudge and dac's telemetry client are set up in dac's `main`
package, so an embedder never runs them. `frontendFS` should be the extracted
`dac-frontend-standalone.tar.gz` for the same dac version.
