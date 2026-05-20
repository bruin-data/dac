# DAC — Dashboard-as-Code

## Design Context

### Users
- **CLI users** (engineers): Technical data/analytics engineers who define dashboards in YAML, run `dac serve` / `dac validate`. Already comfortable with CLI tools and config files.
- **Dashboard viewers** (business): Non-technical business users who consume the rendered dashboards in their browser. They need clarity, not complexity. The UI must serve them — not the engineers who built it.

### Brand Personality
**Precise. Confident. Minimal.**

Bruin is an engineering-grade data tool that presents information with authority. The UI should feel like it knows what it's doing — no unnecessary decoration, no hand-holding, no visual noise. Every pixel earns its place.

### Aesthetic Direction
- **Dense and information-rich** — users want to see everything at once. Favor data density over generous whitespace. Dashboards should feel like a well-organized command center, not a sparse landing page.
- **Data-forward, minimal chrome** — the UI is a frame for the data, not a destination itself. Navigation, controls, and chrome should recede. Charts, numbers, and tables are the stars.
- **Dark mode as a first-class citizen** — both light and dark themes should feel intentional, not like one is an afterthought.

### References
- **Linear** — clean, dense, keyboard-driven, dark mode first
- **Vercel Dashboard** — elegant data displays, polished charts
- **Hex.tech / Observable** — modern data tooling aesthetic, clean data viz

### Anti-References
- **Metabase / Looker** — no heavy enterprise BI chrome, no menu-heavy layouts
- **Generic Bootstrap admin templates** — no template-y card grids, no generic dashboard aesthetic
- **Overly playful / colorful** — no gradients, illustrations, or consumer-app whimsy

### Design Principles
1. **Data is the interface** — every design decision should make the data easier to read, compare, and understand. If a visual element doesn't serve the data, remove it.
2. **Density over decoration** — pack information in. Use tight spacing, compact typography, and efficient layouts. Business users want a full picture, not a scrolling experience.
3. **Quiet confidence** — the UI should feel assured and understated. Subtle borders, restrained color use, confident typography. No need to shout.
4. **Consistency is clarity** — uniform spacing, predictable patterns, consistent component behavior. When everything follows the same rules, nothing needs explanation.
5. **Accessible by default** — practical accessibility: strong contrast ratios, readable font sizes, keyboard-navigable. No WCAG theater, just genuinely usable interfaces.

### Tech Stack
- React 19 + TypeScript + Vite
- Tailwind CSS v4 (utility-first, no component library)
- Recharts for data visualization
- Custom theme system with CSS variables (`--dac-*`)
- Go backend with embedded frontend (single binary)

## Build & Development

**Always use `make` commands** — never run `go build`, `npm run build`, `go test`, or similar directly. The Makefile handles cross-cutting concerns (frontend embedding, CGO flags, correct build order).

| Command | Purpose |
|---|---|
| `make dev` | Run frontend + backend with live reload (for development) |
| `make dev-frontend` | Frontend dev server only (Vite, proxies API to :8321) |
| `make dev-backend` | Backend only with live reload (via `air`) |
| `make build` | Production build: frontend → embed → Go binary |
| `make frontend` | Build frontend assets for Go embedding |
| `make test` | Run all tests |
| `make deps` | Install Go + npm dependencies |
| `make format` | Run `go vet` |
| `make clean` | Remove build artifacts |

When verifying Go changes compile, use `go build ./...` for a quick check, but always use `make build` for a real build.

## Skills

- `/create-dashboard [description]` — Full reference for creating dac dashboard YAML files. Covers widget types (metric, chart, table, text), filters, query templating (Jinja), grid layout, and CLI usage.

## Documentation

After every code change, scan `docs/` for areas the change touches and update them in the same change. If nothing existing fits, add a new doc page. Don't defer docs to a follow-up.
