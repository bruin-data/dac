import { defineConfig } from "vitepress";

export default defineConfig({
  title: "DAC",
  description: "Dashboard-as-Code: define, validate, and serve dashboards from YAML and TSX",
  base: "/dac/",
  themeConfig: {
    outline: "deep",
    search: {
      provider: "local",
    },
    nav: [
      { text: "Guide", link: "/getting-started/installation" },
      { text: "Reference", link: "/dashboards/overview" },
      { text: "Commands", link: "/commands/overview" },
    ],
    sidebar: [
      {
        text: "Getting Started",
        collapsed: false,
        items: [
          { text: "Overview", link: "/" },
          { text: "Installation", link: "/getting-started/installation" },
          { text: "Quickstart", link: "/getting-started/quickstart" },
        ],
      },
      {
        text: "Dashboards",
        collapsed: false,
        items: [
          { text: "Overview", link: "/dashboards/overview" },
          { text: "YAML Format", link: "/dashboards/yaml" },
          { text: "TSX Format", link: "/dashboards/tsx" },
          { text: "Schemas", link: "/dashboards/schemas" },
          { text: "Widgets", link: "/dashboards/widgets" },
          { text: "Filters", link: "/dashboards/filters" },
          { text: "Queries & Templating", link: "/dashboards/queries" },
          { text: "Semantic Layer", link: "/dashboards/semantic-layer" },
          { text: "Layout", link: "/dashboards/layout" },
          { text: "Themes", link: "/dashboards/themes" },
        ],
      },
      {
        text: "Commands",
        collapsed: false,
        items: [
          { text: "Overview", link: "/commands/overview" },
          { text: "init", link: "/commands/init" },
          { text: "serve", link: "/commands/serve" },
          { text: "build", link: "/commands/build" },
          { text: "validate", link: "/commands/validate" },
          { text: "check", link: "/commands/check" },
          { text: "ls", link: "/commands/ls" },
          { text: "query", link: "/commands/query" },
          { text: "connections", link: "/commands/connections" },
          { text: "skills", link: "/commands/skills" },
          { text: "import", link: "/commands/import" },
          { text: "export", link: "/commands/export" },
          { text: "upgrade", link: "/commands/upgrade" },
        ],
      },
      {
        text: "Configuration",
        collapsed: false,
        items: [
          { text: "Connections", link: "/configuration/connections" },
        ],
      },
    ],
    socialLinks: [
      { icon: "github", link: "https://github.com/bruin-data/dac" },
    ],
  },
  markdown: {
    languages: ["sql", "yaml", "shell", "typescript", "tsx", "json"],
  },
});
