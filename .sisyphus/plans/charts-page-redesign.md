# Charts Page Redesign & Main Page Integration

## Problem Statement

The charts page (`/charts/`) has three issues:
1. **Design mismatch** — No search/filter, simpler cards, weaker visual hierarchy vs the main page
2. **Broken image links** — Chart image overrides link to `/catalog/{name}/`, but catalog pages only exist for images in `full-catalog.ts`. Any chart image not in that file → 404
3. **Chart images missing from main page** — User wants ALL images (including chart-specific ones) visible on the main page, while keeping `/charts/` as a dedicated organizational page

## Root Cause Analysis

### Broken links
- `catalog/[...name].astro` generates pages via `getStaticPaths()` from `fullCatalog` (in `full-catalog.ts`)
- Charts page links image overrides to `/catalog/${patchedRefToName(m.patchedRepo)}/`
- If a chart image override references an image NOT in `full-catalog.ts`, there's no generated page → 404
- Currently some prometheus chart images (k8s-sidecar, configmap-reload, kube-state-metrics) may already exist in `full-catalog.ts` — must be verified by cross-referencing
- But victoria-logs, postgres-operator chart images may not be present → potential 404s as charts grow

### Design gap
- Main page uses `ImageCatalog.astro` with search input, source filter buttons (All/Wolfi-Based/Patched), category grouping, and responsive grid rows
- Charts page builds its own card layout from scratch — no search, no filtering, collapsible image overrides tables
- Visually: charts page uses `border-verity-nucleus/30` summary cards vs main page's `border-verity-border` + `hover:border-verity-orbit` pattern

---

## Plan

### Task 1: Add missing chart images to `full-catalog.ts`

**File:** `site/src/data/full-catalog.ts`

Cross-reference every `imageMappings[].patchedRepo` in `charts-catalog.json` against `full-catalog.ts`. Any missing image gets added to the appropriate category. This:
- Generates catalog detail pages (fixes broken links)
- Automatically surfaces them in the main page's ImageCatalog

For images that only appear as chart overrides (e.g. `victoriametrics/victoria-logs` from the victoria-logs chart), add them to the most fitting category. If no category fits, create a new one or add to "Monitoring & Observability".

**QA:**
1. Run `npx astro check` in `site/` — zero errors
2. Run `lsp_diagnostics` on `site/src/data/full-catalog.ts` — zero errors
3. Run `npm run build` in `site/` — exit code 0, no broken link warnings
4. For each `imageMappings[].patchedRepo` in `charts-catalog.json`, confirm a matching entry exists in `full-catalog.ts` by grepping for the extracted name

### Task 2: Redesign charts page to match main page patterns

**File:** `site/src/pages/charts/index.astro`

#### 2a. Add search + filter

Port the search/filter pattern from `ImageCatalog.astro`:
- Search input: `Search {n} charts...` — filters by chart name, image names in overrides, version
- Add `data-searchable` attributes to chart cards containing searchable text
- Reuse the same input styling: `w-full sm:w-96 px-3 py-2 border border-verity-border rounded-lg text-sm bg-verity-surface ...`
- Place search bar above the "Available Charts" section

No source filter needed — charts don't have copa/integer distinction. But if chart count grows, consider filtering by chart name or image count.

#### 2b. Match summary cards to main page stats pattern

Current: `grid-cols-2 sm:grid-cols-3` with `border-verity-nucleus/30`
Target: Match the main page HeroSection's stat cards — `grid-cols-2 sm:grid-cols-4` with `border-verity-border` + `hover:border-verity-orbit transition-colors`

Add a 4th stat card (e.g. "Total CVEs Patched" aggregated across all chart overrides).

#### 2c. Match chart card design to main page conventions

Current chart cards use collapsible image overrides (hidden by default behind a toggle). This is less discoverable.

Changes:
- **Remove collapsible toggle** — show image overrides inline (same as how main page shows all image rows openly)
- **Use consistent borders**: `border-verity-border` + `hover:border-verity-orbit` (not `border-verity-border/50`)
- **Match section header pattern**: reuse the `flex items-center gap-3 mb-4 pb-2 border-b border-verity-border` pattern from ImageCatalog categories
- **Image override rows**: Use the same responsive grid pattern as ImageCatalog rows: `grid grid-cols-1 sm:grid-cols-[1fr_7.5rem_4.5rem_1fr]` with columns: Original → Type Badge → CVEs → Patched Ref

#### 2d. Fix image override links

Currently correct (`<a href={base}catalog/${imgName}/ ...>`) but depends on Task 1 to ensure target pages exist. After Task 1, all links will resolve.

Additionally: make the entire image override row clickable (like ImageCatalog rows are full `<a>` tags), not just the patched ref text.

#### 2e. Client-side search script

Port the `applyFilters()` pattern from `ImageCatalog.astro`:
```js
// Search across chart name, version, image names, install command
const query = input.value.toLowerCase();
document.querySelectorAll('.chart-card').forEach(card => {
  const text = card.getAttribute('data-searchable') ?? '';
  card.style.display = text.includes(query) ? '' : 'none';
});
```

**QA:**
1. `lsp_diagnostics` on `site/src/pages/charts/index.astro` — zero errors
2. `grep 'catalog-search\|chart-search' site/src/pages/charts/index.astro` — search input element present
3. `grep 'data-searchable' site/src/pages/charts/index.astro` — searchable attributes on chart cards
4. `grep 'hover:border-verity-orbit' site/src/pages/charts/index.astro` — main-page hover pattern present (not `border-verity-nucleus/30`)
5. `grep 'SourceBadge' site/src/pages/charts/index.astro` — SourceBadge component imported and used
6. `npm run build` in `site/` — exit code 0

### Task 3: Add chart images to main page Image Catalog

This is **automatically handled by Task 1** — adding chart images to `full-catalog.ts` makes them appear in `ImageCatalog.astro` on the main page.

No additional code changes needed on `index.astro`. The `ImageCatalog` component iterates `fullCatalog` and renders all entries.

**QA:** `npm run build` in `site/` succeeds and `grep` for the newly added image names in `site/dist/index.html` confirms they appear on the main page.

### Task 4: Final verification

**QA (all executable):**
1. `npm run build` in `site/` — exit code 0, zero broken link warnings in stdout
2. `lsp_diagnostics` on all modified files — zero errors:
   - `site/src/data/full-catalog.ts`
   - `site/src/pages/charts/index.astro`
3. `grep -c 'data-searchable' site/src/pages/charts/index.astro` — returns ≥1 (search attributes present)
4. `grep -c 'catalog-search\|chart-search' site/src/pages/charts/index.astro` — returns ≥1 (search input exists)
5. Confirm chart image override links exist in build output: for each chart image, `ls site/dist/catalog/{imgName}/index.html` returns 0
6. Confirm chart images appear on main page: `grep '{imageName}' site/dist/index.html` for each added image

---

## Files Modified

| File | Change |
|------|--------|
| `site/src/data/full-catalog.ts` | Add missing chart images to categories |
| `site/src/pages/charts/index.astro` | Full redesign — search, card layout, inline overrides, matched styling |

## Files NOT Modified (reused as-is)

| File | Why |
|------|-----|
| `site/src/components/ImageCatalog.astro` | No changes — already handles all images from full-catalog |
| `site/src/components/SeverityBadges.astro` | Already used on charts page |
| `site/src/components/SourceBadge.astro` | Will now be imported on charts page for image override rows |
| `site/src/pages/index.astro` | No changes — chart images auto-appear via ImageCatalog |
| `site/src/pages/catalog/[...name].astro` | No changes — auto-generates pages for new full-catalog entries |

## Design Tokens Reference (from main page)

```
Search input:  w-full sm:w-96 px-3 py-2 border border-verity-border rounded-lg text-sm bg-verity-surface text-verity-text-primary placeholder-verity-text-secondary focus:outline-hidden focus:ring-2 focus:ring-verity-nucleus
Stat card:     bg-verity-surface border border-verity-border rounded-lg p-4 text-center hover:border-verity-orbit transition-colors
Section head:  flex items-center gap-3 mb-4 pb-2 border-b border-verity-border
Category pill: text-xs px-2 py-0.5 rounded-full bg-verity-surface border border-verity-border text-verity-text-muted
Image row:     grid grid-cols-1 sm:grid-cols-[1fr_7.5rem_4.5rem_1fr] gap-x-4 gap-y-1 items-center px-3 py-3 border-b border-verity-border/50 hover:bg-verity-surface/80
```
