# Installation

DAC is released as standalone binaries through GitHub Releases.

## Install Script

```shell
curl -fsSL https://raw.githubusercontent.com/bruin-data/dac/main/install.sh | bash
```

The installer installs the Bruin CLI first if `bruin` is not already available on your `PATH`, then downloads the latest DAC GitHub release for your platform and installs `dac` into `~/.local/bin` by default.

DAC uses `.bruin.yml` connections and currently shells out to `bruin query` to execute dashboard SQL.

To install the latest edge build from `main`:

```shell
curl -fsSL https://raw.githubusercontent.com/bruin-data/dac/main/install.sh | bash -s -- --channel edge
```

To install into a different directory:

```shell
curl -fsSL https://raw.githubusercontent.com/bruin-data/dac/main/install.sh | bash -s -- -b /usr/local/bin
```

To install a specific version:

```shell
curl -fsSL https://raw.githubusercontent.com/bruin-data/dac/main/install.sh | bash -s -- v0.1.0
```

## Upgrading

Once `dac` is on your `PATH`, upgrade in place with:

```shell
dac upgrade
```

This re-runs the official installer to replace the current binary with the latest release on the same channel. See [`dac upgrade`](/commands/upgrade) for flags, channel switching, and pinning a specific version.

DAC also checks GitHub once a day for newer releases on your channel and prints a one-line notice to stderr when one is available. Set `DAC_NO_UPDATE_CHECK=1` or `DO_NOT_TRACK=1` to disable.

## Build from Source

If you are developing DAC itself:

```shell
git clone https://github.com/bruin-data/dac.git
cd dac
make deps
make build
```

The binary is output under the repository `bin/` directory. Add that directory to your `PATH` if you want to run it as `dac`.

## Verify Installation

```shell
dac version
```

If you will run dashboards locally, also verify that the Bruin CLI is on your `PATH`:

```shell
bruin --version
```
