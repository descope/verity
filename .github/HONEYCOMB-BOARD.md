# Verity Patch Pipeline — Honeycomb Board Setup

Manual board setup for the `verity-ci` dataset (env: `verity-prod`). Honeycomb's
`Run Queries` API requires an enterprise plan, so the queries below must be
created via the UI.

## Prerequisites

- At least one patch run has emitted spans to `verity-ci` (via `patch-image.yaml`)
- Verify the dataset exists: Honeycomb → Datasets → `verity-ci`

## Steps

1. Open Honeycomb → **Boards** → **New Board** → name it `Verity Patch Pipeline`
2. For each query below: **Add Query** → paste the JSON into the URL trick
   (see "Pasting via URL" below) or rebuild it field-by-field in the query builder
3. Drag panels into the layout described under "Layout"

## Pasting via URL (fastest)

Honeycomb supports pasting JSON queries via the query builder URL. Append
`?query=<URL-encoded-JSON>` to any dataset's query page:

```
https://ui.honeycomb.io/<team>/environments/verity-prod/datasets/verity-ci/result?query=<JSON>
```

Replace `<team>` with `omer-c6` (your team slug).

## The six queries

### 1. Patch Duration p50/p95 (HEATMAP)

```json
{
  "calculations": [
    { "op": "HEATMAP", "column": "copa_duration_seconds" },
    { "op": "P50", "column": "copa_duration_seconds" },
    { "op": "P95", "column": "copa_duration_seconds" }
  ],
  "filters": [{ "column": "name", "op": "=", "value": "patch-image.matrix" }],
  "time_range": 604800
}
```

Display as **graph**. Heatmap cells with p50/p95 lines overlaid.

### 2. CVE Burndown

```json
{
  "calculations": [
    { "op": "SUM", "column": "cve_before" },
    { "op": "SUM", "column": "cve_after" }
  ],
  "filters": [{ "column": "name", "op": "=", "value": "patch-image.matrix" }],
  "time_range": 604800
}
```

Display as **graph**. Two stacked lines showing total CVEs eliminated.

### 3. Slowest Images (top 10)

```json
{
  "calculations": [{ "op": "AVG", "column": "copa_duration_seconds" }],
  "filters": [{ "column": "name", "op": "=", "value": "patch-image.matrix" }],
  "breakdowns": ["image"],
  "orders": [{ "op": "AVG", "column": "copa_duration_seconds", "order": "descending" }],
  "limit": 10,
  "time_range": 604800
}
```

Display as **table**. Top 10 slowest images by AVG patch duration.

### 4. Failures by Image (`copa_exit != 0`)

```json
{
  "calculations": [{ "op": "COUNT" }],
  "filters": [
    { "column": "name", "op": "=", "value": "patch-image.matrix" },
    { "column": "copa_exit", "op": "!=", "value": 0 }
  ],
  "breakdowns": ["image"],
  "time_range": 604800
}
```

Display as **combo**. Count of failed Copa runs grouped by image.

### 5. Patches by Platform (amd64 vs arm64)

```json
{
  "calculations": [{ "op": "COUNT" }],
  "filters": [{ "column": "name", "op": "=", "value": "patch-image.matrix" }],
  "breakdowns": ["platform"],
  "time_range": 604800
}
```

Display as **combo**. Stacked bar by platform showing volume parity.

### 6. Residual CVEs by Image (post-scan)

```json
{
  "calculations": [{ "op": "AVG", "column": "post_scan_vuln_count" }],
  "filters": [{ "column": "name", "op": "=", "value": "patch-image.finalize" }],
  "breakdowns": ["image"],
  "orders": [{ "op": "AVG", "column": "post_scan_vuln_count", "order": "descending" }],
  "limit": 25,
  "time_range": 604800
}
```

Display as **table**. AVG residual vuln count after patch, top 25.

## Layout

```
┌─────────────────────────────────────────────────┐
│ # Verity Patch Pipeline (text panel)            │
├─────────────────────────────────────────────────┤
│ 1. Patch Duration p50/p95 HEATMAP (full width)  │
├──────────────────────────┬──────────────────────┤
│ 2. CVE Burndown          │ 4. Failures by Image │
├──────────────────────────┼──────────────────────┤
│ 5. Patches by Platform   │ 3. Slowest Images    │
├──────────────────────────┴──────────────────────┤
│ 6. Residual CVEs by Image (full width, table)   │
└─────────────────────────────────────────────────┘
```

## Span attributes reference

Emitted by `.github/workflows/patch-image.yaml` via `otel-cli`:

| Span name | Attributes |
|---|---|
| `patch-image.matrix` | `image, platform, cve_before, cve_after, package_list_sha256, rekor_url, copa_exit, copa_duration_seconds, staging_digest` |
| `patch-image.finalize` | `image, manifest_digest, post_scan_vuln_count, attestation_id, attestation_bundle_path` |

Both spans share resource attributes: `service.name=verity-ci`, `deployment.environment=verity-prod`.
