# dac upgrade

Upgrade the DAC CLI in place to the latest release.

```shell
dac upgrade [tag] [flags]
```

`dac upgrade` re-runs the official install script to replace the current binary with the latest release on the same channel. The channel is inferred from the running binary's version, so stable installs stay on stable and edge installs stay on edge.

`dac update` is an alias.

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--channel` | string | inferred from current version | Release channel to install from (`stable` or `edge`) |
| `--bindir` | string | directory of the current binary | Directory to install into |

## Examples

```shell
# Upgrade to the latest release on the current channel
dac upgrade

# Switch to the edge channel
dac upgrade --channel edge

# Pin to a specific version
dac upgrade v0.1.0

# Install into a different directory (may require sudo)
dac upgrade --bindir /usr/local/bin
```

## Update Notifications

DAC checks GitHub once per day for newer releases on the channel you installed from. When one is available, a one-line notice is printed to stderr after the command finishes:

```text
dac v0.2.0 available (current: v0.1.0). Run `dac upgrade`.
```

The notice is skipped when stderr is not a terminal, so scripts and CI logs stay clean. The check itself runs in the background and never blocks the command path.

To disable the check entirely:

```shell
export DAC_NO_UPDATE_CHECK=1
# or the industry-standard:
export DO_NOT_TRACK=1
```

The check is also disabled for `dev` and `test-*` builds.
