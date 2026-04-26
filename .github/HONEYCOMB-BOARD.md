# Verity Patch Pipeline — Honeycomb Board

**Live board**: https://ui.honeycomb.io/omer-c6/environments/verity-prod/board/89dYstv3emD

The board is a one-time hand-provisioned artifact against the `verity-ci`
dataset (env: `verity-prod`) with 7 query panels covering all Phase 1 SLOs.
It was created during PR #262 development by POSTing the JSON payloads
embedded in the "Re-creating from scratch" section below to Honeycomb's
Boards/Queries API. The ingest key needs `boards: true` + `columns: true`
(creating and saving queries) — `Run Queries` (executing queries via API)
is enterprise-only, but query *creation* is not.

## Panels

| Panel | Type | Calculation | Span |
|---|---|---|---|
| Patch Duration p50/p95 | HEATMAP + overlays | `copa_duration_seconds` | matrix |
| CVEs Before Patch | line | `SUM(cve_before)` over time | matrix |
| CVEs After Patch | line | `SUM(post_scan_vuln_count)` over time | finalize |
| Failures by Image | combo | `COUNT WHERE copa_exit != 0` group by `image` | matrix |
| Patches by Platform | combo | `COUNT` group by `platform` | matrix |
| Slowest Images | table | top 10 `AVG(copa_duration_seconds)` by `image` | matrix |
| Residual CVEs by Image | table | top 25 `AVG(post_scan_vuln_count)` by `image` | finalize |

CVE Burndown is two side-by-side panels (before vs after) because the data
lives on different span types — matrix has pre-patch counts (`cve_before`),
finalize has post-patch counts (`post_scan_vuln_count`). Honeycomb cannot
combine columns from different spans in a single calculation.

## Span attributes reference

Emitted by `.github/workflows/patch-image.yaml` via `otel-cli`:

| Span name | Attributes |
|---|---|
| `patch-image.matrix` | `image`, `platform`, `cve_before`, `package_list_sha256`, `copa_exit`, `copa_duration_seconds`, `staging_digest` |
| `patch-image.finalize` | `image`, `manifest_digest`, `post_scan_vuln_count`, `rekor_url`, `attestation_id`, `attestation_bundle_path` |

Resource attributes on every span: `service.name=verity-ci`, `deployment.environment=verity-prod`.

## Re-creating from scratch

If the board is deleted or you need to recreate it in another environment,
the queries and board can be POSTed to the Honeycomb API directly. The
ingest key needs at least `boards: true` and `columns: true`.

The 3-step API flow:

1. `POST /1/queries/{dataset}` — create each query → returns `query_id`
2. `POST /1/query_annotations/{dataset}` with `{query_id, name, description}` → returns `annotation_id`
3. `POST /1/boards` with `panels: [{type:"query", query_panel:{query_id, query_annotation_id, query_style}}]`

### Query payloads (paste into `POST /1/queries/verity-ci`)

**Patch Duration p50/p95** (graph)

```json
{
  "calculations": [
    {"op": "HEATMAP", "column": "copa_duration_seconds"},
    {"op": "P50",     "column": "copa_duration_seconds"},
    {"op": "P95",     "column": "copa_duration_seconds"}
  ],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.matrix"}],
  "time_range": 604800
}
```

**CVEs Before Patch** (graph)

```json
{
  "calculations": [{"op": "SUM", "column": "cve_before"}],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.matrix"}],
  "time_range": 604800
}
```

**CVEs After Patch** (graph)

```json
{
  "calculations": [{"op": "SUM", "column": "post_scan_vuln_count"}],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.finalize"}],
  "time_range": 604800
}
```

**Slowest Images** (table)

```json
{
  "calculations": [{"op": "AVG", "column": "copa_duration_seconds"}],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.matrix"}],
  "breakdowns": ["image"],
  "orders": [{"op": "AVG", "column": "copa_duration_seconds", "order": "descending"}],
  "limit": 10,
  "time_range": 604800
}
```

**Failures by Image** (combo)

```json
{
  "calculations": [{"op": "COUNT"}],
  "filters": [
    {"column": "name", "op": "=", "value": "patch-image.matrix"},
    {"column": "copa_exit", "op": "!=", "value": 0}
  ],
  "breakdowns": ["image"],
  "time_range": 604800
}
```

**Patches by Platform** (combo)

```json
{
  "calculations": [{"op": "COUNT"}],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.matrix"}],
  "breakdowns": ["platform"],
  "time_range": 604800
}
```

**Residual CVEs by Image** (table)

```json
{
  "calculations": [{"op": "AVG", "column": "post_scan_vuln_count"}],
  "filters": [{"column": "name", "op": "=", "value": "patch-image.finalize"}],
  "breakdowns": ["image"],
  "orders": [{"op": "AVG", "column": "post_scan_vuln_count", "order": "descending"}],
  "limit": 25,
  "time_range": 604800
}
```
