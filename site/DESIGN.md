# DESIGN.md — Verity Supply

## Purpose
Security supply-chain catalog browser and trust documentation for platform engineers and DevSecOps teams. Not a marketing page. Data precision, pipeline automation, and verifiable attestations are the visual story.

## Aesthetic Direction
**Industrial Terminal** — deep void backgrounds, monospace-first, tight uppercase labels, teal (`#00f0cc`) as the only warm accent. The site feels like a hardened ops dashboard that runs at 02:00 UTC without supervision.

Anti-patterns for this project:
- No purple gradients on white
- No rounded consumer cards (`rounded-2xl` everywhere)
- No proportional sans-serif headings
- No hero illustration or lifestyle imagery
- No CTA-first marketing layout that buries the catalog

## Color Tokens (`@theme` in `src/styles/global.css`)

| Token | Hex | Use |
|---|---|---|
| `verity-nucleus` | `#00f0cc` | CTAs, active states, trust accent — use sparingly |
| `verity-orbit` | `#0099a8` | Links, secondary accent |
| `verity-void` | `#060d12` | Page background |
| `verity-surface` | `#0a1720` | Card / panel background |
| `verity-surface-2` | `#111b22` | Hover lift |
| `verity-border` | `#152230` | Primary border |
| `verity-border-2` | `#1f2b35` | Secondary / emphasis border |
| `verity-text` | `#e8edf2` | Primary text (headings) |
| `verity-text-primary` | `#c5d5dd` | Standard readable text |
| `verity-text-secondary` | `#95a8b8` | Supporting / nav text (WCAG AA ✓) |
| `verity-text-muted` | `#7a8e9c` | Labels, metadata (WCAG AA ✓) |

Severity palette: critical `#f87171`, high `#fb923c`, medium `#facc15`, low `#4ade80`.
Amber used for experimental / warning states.

## Typography

- **UI / brand voice**: `Share Tech Mono` (`var(--font-mono)`) — the entire page is monospace. This is intentional.
- **Code / data**: `JetBrains Mono` (`var(--font-code)`) — image refs, pull commands, code blocks.
- No proportional sans-serif anywhere.

### Scale
| Role | Size | Tracking | Case |
|---|---|---|---|
| Hero h1 | `text-4xl` → `text-[52px]` | `0.12em` | UPPERCASE |
| Section h2 | `text-xl font-semibold` | `tracking-wider` | Title |
| Category label | `text-sm tracking-wider` | — | UPPERCASE |
| System label | `text-[11px]` | `0.25em` | UPPERCASE `text-verity-text-muted` |
| Body | `text-sm` – `text-[15px]` | — | `leading-[1.7]` |
| Code/mono | `text-xs` – `text-[13px]` | — | `font-[var(--font-code)]` |

## Spacing

4 px base grid. Use Tailwind's spacing scale; custom vars `--s-1` (4 px) through `--s-9` (96 px) for
reference.

- Section gaps: `mb-10` – `mb-16`
- Card padding: `p-4` – `p-5`
- Row gaps: `gap-3` – `gap-4`

## Component Rules

### Borders & Radii
- Card: `border border-verity-border rounded` (4 px)
- Panel / multi-step: `border border-verity-border-2 rounded-lg` (6 px)
- Active/highlighted card: `border-verity-nucleus shadow-[0_0_20px_rgba(0,240,204,0.25)]`
- Pill badge: `rounded-full`

### Buttons
- **Primary CTA**: `bg-verity-nucleus text-[#002820]` + nucleus glow shadow on hover
- **Secondary / ghost**: `border border-verity-border-2 text-verity-text`
- **Focus-visible**: `outline-2 outline-offset-2 outline-verity-nucleus` (set globally in `global.css`)

### Terminal Blocks
- Body: `bg-black` or `bg-verity-void`
- Header bar: `bg-verity-surface border-b border-verity-border`
- Window chrome dots: decorative, `aria-hidden="true"`
- Font: `font-[var(--font-code)] text-[13px]`
- Prompt `$`: `text-verity-nucleus`

### Filter Controls
- Container: `bg-verity-surface border border-verity-border rounded-lg`, `role="group"` + `aria-label`
- Active button: `bg-verity-nucleus text-[#002820]`
- Inactive button: `text-verity-text-secondary hover:bg-verity-void`
- `aria-pressed` attribute must be managed by JS

### Catalog Rows
- Grid: `grid-cols-[1fr_7.5rem_4.5rem_1fr]` (sm+), single-column on mobile
- Row separator: `border-b border-verity-border/50`
- Hover: `hover:bg-verity-surface/80`
- Each row links to the catalog detail page

## Motion & Accessibility

### Motion Rules
- Animate **`transform` / `opacity` / `filter` only** — never animate layout properties (width, height, padding, margin)
- Durations: `120ms` (micro) · `180ms` (standard) · `280ms` (emphasis)
- Easing: `cubic-bezier(0.2, 0.7, 0.2, 1)` for entrances; `cubic-bezier(0.4, 0, 0.2, 1)` for toggles
- All CSS animations + transitions respect `prefers-reduced-motion: reduce` (global override in `global.css`)

### Accessibility Checklist
- WCAG AA contrast on all text tokens verified (see comments in `global.css`)
- Skip link to `#main-content` present in `BaseLayout.astro`
- All interactive elements have visible `:focus-visible` ring (global default in `global.css`)
- `aria-label` on icon-only buttons
- `aria-pressed` on toggle/filter buttons, managed by JS
- `role="group"` + `aria-label` on filter button sets
- `role="list"` / `role="listitem"` on pipeline step sequences
- Decorative SVGs: `aria-hidden="true"`
- `aria-current="page"` on active nav links

### Scrollbar
Custom scrollbar (`global.css`): `verity-void` track, `verity-border-2` thumb, `verity-nucleus` on hover.

### Text Selection
`::selection` teal tint: `rgba(0, 240, 204, 0.18)` background, `#e8edf2` foreground.
