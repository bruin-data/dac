# Themes

DAC includes a theme system based on design tokens. Choose a built-in theme or define your own.

## Built-in Themes

| Theme | Description |
|-------|-------------|
| `bruin` | Light theme (default) |
| `bruin-dark` | Dark theme |

Select the theme with the `--template` flag, which applies to all dashboards:

```shell
dac serve --template bruin-dark
```

## Custom Themes

Create a YAML file with your theme tokens:

```yaml
name: corporate
extends: bruin

tokens:
  background: "#FFFFFF"
  surface: "#F8F9FA"
  accent: "#0066CC"
  text-primary: "#1A1A1A"
  text-secondary: "#6B7280"
  border: "#E5E7EB"
```

Use it:

```shell
dac serve --template ./themes/corporate.yml
```

### Token Reference

Themes use CSS custom properties prefixed with `--dac-*`. The tokens you define in YAML are mapped to these variables and applied to the frontend.

## Theme Directory

Place theme files in a `themes/` directory alongside your dashboards. DAC discovers them automatically and makes them available by name:

```
dashboards/
├── sales.yml
├── charts.yml
└── themes/
    ├── corporate.yml
    └── minimal.yml
```

```shell
dac serve --template corporate
```

## Runtime Theme Switching

The dashboard viewer includes a theme toggle in the UI that switches between light and dark modes. The `--template` flag sets the initial theme.
