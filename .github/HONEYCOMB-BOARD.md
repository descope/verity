# Verity Patch Pipeline — Honeycomb Board

**Live board**: https://ui.honeycomb.io/omer-c6/environments/verity-prod/board/89dYstv3emD

The board is auto-created against the `verity-ci` dataset (env: `verity-prod`)
with 7 query panels covering all Phase 1 SLOs. Created via the Honeycomb
Boards/Queries API. The ingest key has `boards: true` + `columns: true` (which
also covers query creation) — `Run Queries` (executing queries via API) is
enterprise-only, but *creating and saving* queries is not.

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
| `patch-image.matrix` | `image`, `platform`, `cve_before`, `package_list_sha256`, `rekor_url`, `copa_exit`, `copa_duration_seconds`, `staging_digest` |
| `patch-image.finalize` | `image`, `manifest_digest`, `post_scan_vuln_count`, `attestation_id`, `attestation_bundle_path` |

Resource attributes on every span: `service.name=verity-ci`, `deployment.environment=verity-prod`.

## Re-creating from scratch

If the board is deleted or you need to recreate it in another environment,
the queries and board can be POSTed to the Honeycomb API directly. The
ingest key needs at least `boards: true` and `columns: true`.

The 3-step API flow:

1. `POST /1/queries/{dataset}` — create each query → returns `query_id`
2. `POST /1/query_annotations/{dataset}` with `{query_id, name, description}` → returns `annotation_id`
3. `POST /1/boards` with `panels: [{type:"query", query_panel:{query_id, query_annotation_id, query_style}}]`

See PR #262 commit history for the JSON payloads of each query.
