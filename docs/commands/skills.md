# skills

List and install DAC agent skills into a project.

Agent skills are local instructions that help coding agents create and modify DAC dashboards correctly. DAC currently ships a bundled `create-dashboard` skill for dashboard authoring.

`dac init` installs bundled skills automatically for new projects. Use `dac skills install` for existing projects, or `dac skills update` to refresh installed skills to the version bundled with your current `dac` binary.

## List Skills

```shell
dac skills list
```

This prints the bundled skills and their install targets.

## Install Skills

```shell
dac skills install --dir .
```

By default, this installs every bundled DAC skill into the target project:

```text
.claude/skills/create-dashboard/SKILL.md
.codex/skills/create-dashboard -> ../../.claude/skills/create-dashboard
```

Claude gets the real skill file. Codex gets a symlink to the same skill directory so both agents use one shared copy.

Install a specific skill:

```shell
dac skills install create-dashboard --dir .
```

Overwrite an existing customized skill:

```shell
dac skills install create-dashboard --dir . --force
```

Restart your agent session after installing skills so the agent can discover the new local instructions.

## Update Skills

Each bundled skill carries a version. When you upgrade `dac`, the binary may ship a newer skill than the copy already installed in your project. `dac serve` and `dac validate` print a one-line notice when a newer skill is available:

```text
A newer create-dashboard skill is available with the latest authoring features and fixes.
Run `dac skills update` to update your project's copy.
```

The notice never modifies files on its own — updating is always an explicit action. Refresh installed skills to the bundled version:

```shell
dac skills update --dir .
```

Update a specific skill:

```shell
dac skills update create-dashboard --dir .
```

`update` overwrites the installed skill files (equivalent to `install --force`), so any local customizations are replaced.

## Flags

| Flag | Alias | Description |
|------|-------|-------------|
| `--dir` | `-d` | Target project directory |
| `--force` | `-f` | Overwrite existing skill files (`install` only; `update` always overwrites) |

## Notes

- Installing skills does not change dashboard behavior at runtime.
- Skills are optional; they improve agent authoring and review workflows.
- Existing files are not overwritten unless `--force` is provided.
