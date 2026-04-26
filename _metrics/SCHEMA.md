# Verity Metrics Schema

## Overview

The `_metrics` orphan branch serves as a durable archive of per-run patch metrics, used for long-term trend analysis including Patch Lag SLO tracking and CVE Burndown visualization. These metrics power dashboards on verity.supply. Metrics are emitted by `.github/workflows/metrics-finalize.yaml` and consumed by the `verity metrics` Go subcommand (Phase 4).

## Path Layout

```
_metrics/
├── README.md
├── SCHEMA.md (this file)
└── runs/
    └── YYYY-MM-DD/
        └── <run-id>-attempt-<run-attempt>/
            └── <image-safe-name>.json
```

- `<run-id>` = GitHub Actions workflow run ID of the patch-image run
- `<run-attempt>` = the run-attempt counter (1, 2, … on retries)
- `<image-safe-name>` = image name with `/`, `:`, and ` ` replaced by `-` (matches the `tr '/: ' '---'` transform in the `scan` job's `parse` step)
- Aggregation chooses LATEST attempt per run-id when multiple exist

## JSON Schema (per-image, per-run)

```json
{
  "schema_version": "v1",
  "run": {
    "id": "<int>",
    "attempt": "<int>",
    "started_at": "<ISO-8601>",
    "ended_at": "<ISO-8601>",
    "conclusion": "success | failure | cancelled | skipped"
  },
  "image": {
    "name": "<string>",
    "source_tag": "<string>",
    "target_ref": "<string>",
    "manifest_digest": "<sha256:... | null>"
  },
  "scan": {
    "before": {
      "vuln_count": "<int>",
      "by_severity": {
        "CRITICAL": "<int>",
        "HIGH": "<int>",
        "MEDIUM": "<int>",
        "LOW": "<int>",
        "UNKNOWN": "<int>"
      }
    },
    "after": {
      "vuln_count": "<int>",
      "by_severity": {
        "CRITICAL": "<int>",
        "HIGH": "<int>",
        "MEDIUM": "<int>",
        "LOW": "<int>",
        "UNKNOWN": "<int>"
      }
    }
  },
  "platforms": {
    "amd64": {
      "arch": "amd64",
      "copa_duration_seconds": "<int | null>",
      "copa_exit_code": "<int | null>",
      "staging_digest": "<sha256:... | null>"
    },
    "arm64": {
      "arch": "arm64",
      "copa_duration_seconds": "<int | null>",
      "copa_exit_code": "<int | null>",
      "staging_digest": "<sha256:... | null>"
    }
  },
  "supply_chain": {
    "rekor_url": "<URL | null>",
    "attestation_id": "<string | null>",
    "sbom_digest": "<sha256:... | null>",
    "attestation_bundle_path": "<string | null>"
  }
}
```

### Field Reference

| Field | Type | Nullable | Notes |
|-------|------|----------|-------|
| `schema_version` | string | No | Schema version, currently "v1" |
| `run.id` | integer | No | GitHub Actions workflow run ID |
| `run.attempt` | integer | No | Run attempt number (1, 2, ...) |
| `run.started_at` | string (ISO-8601) | Yes | Workflow start timestamp; empty string when the GitHub API lookup failed |
| `run.ended_at` | string (ISO-8601) | No | Workflow end timestamp |
| `run.conclusion` | string | No | One of: success, failure, cancelled, skipped |
| `image.name` | string | No | Full image name (e.g., "nginx") |
| `image.source_tag` | string | No | Source tag being patched (e.g., "1.25-alpine") |
| `image.target_ref` | string | No | Target reference for patched image |
| `image.manifest_digest` | string | Yes | sha256 digest of the manifest; null if scan failed |
| `scan.before` | object | Yes | Null when conclusion is "failure" (metrics-on-failure path) |
| `scan.before.vuln_count` | integer | No | Total vulnerabilities before patching |
| `scan.before.by_severity.CRITICAL` | integer | No | Critical severity count before |
| `scan.before.by_severity.HIGH` | integer | No | High severity count before |
| `scan.before.by_severity.MEDIUM` | integer | No | Medium severity count before |
| `scan.before.by_severity.LOW` | integer | No | Low severity count before |
| `scan.before.by_severity.UNKNOWN` | integer | No | Unknown severity count before |
| `scan.after` | object | Yes | Null when conclusion is "failure" (no post-scan happened) |
| `scan.after.vuln_count` | integer | No | Total vulnerabilities after patching |
| `scan.after.by_severity.CRITICAL` | integer | No | Critical severity count after |
| `scan.after.by_severity.HIGH` | integer | No | High severity count after |
| `scan.after.by_severity.MEDIUM` | integer | No | Medium severity count after |
| `scan.after.by_severity.LOW` | integer | No | Low severity count after |
| `scan.after.by_severity.UNKNOWN` | integer | No | Unknown severity count after |
| `platforms.amd64` | object | Yes | Null when amd64 filtered via platforms input |
| `platforms.amd64.arch` | string | No | Always `"amd64"` when the object is non-null |
| `platforms.amd64.copa_duration_seconds` | integer | Yes | Duration of Copa patching; null if skipped/failed |
| `platforms.amd64.copa_exit_code` | integer | Yes | Copa exit code; null if skipped |
| `platforms.amd64.staging_digest` | string | Yes | Staging image digest; null if build failed |
| `platforms.arm64` | object | Yes | Null when arm64 filtered via platforms input |
| `platforms.arm64.arch` | string | No | Always `"arm64"` when the object is non-null |
| `platforms.arm64.copa_duration_seconds` | integer | Yes | Duration of Copa patching; null if skipped/failed |
| `platforms.arm64.copa_exit_code` | integer | Yes | Copa exit code; null if skipped |
| `platforms.arm64.staging_digest` | string | Yes | Staging image digest; null if build failed |
| `supply_chain.rekor_url` | string (URL) | Yes | Rekor transparency log URL; null if not attested |
| `supply_chain.attestation_id` | string | Yes | Attestation identifier; null if not attested |
| `supply_chain.sbom_digest` | string | Yes | SBOM digest; null if SBOM generation failed |
| `supply_chain.attestation_bundle_path` | string | Yes | Path to attestation bundle; null if not generated |

All fields marked nullable may be null when the upstream step was skipped or failed.

## Retention Policy

- Phase 1: no automated pruning; operator review at 90 days
- Phase 2 (deferred): quarterly compaction job rolls up >90-day data into monthly summaries
- All commits preserved (orphan branch, not history-rewriting)

## Aggregation Policy

- Daily aggregate produced by `metrics-aggregate.yaml` (planned 05:30 UTC, after orchestrator completes by ~02:15) — outputs `_metrics/daily/YYYY-MM-DD/summary.json` (DEFERRED to Phase 2)
- Site rendering: `verity metrics` Go subcommand reads `_metrics/runs/**` (latest attempt per run-id, last 90 days) and emits `site/src/data/metrics.json` for Astro's static-import pattern (mirrors existing `verity catalog` precedent)

## Versioning

- Schema version: `v1`
- Add `"schema_version": "v1"` field at top of each JSON file
- Breaking changes increment major (v2), accompanied by migration script

## Producer / Consumer

- **Producer**: `.github/workflows/patch-image.yaml` (final consolidate step), via `metrics-finalize.yaml` (workflow_run trigger)
- **Consumer**: `verity metrics` subcommand (Phase 4); Honeycomb dataset `verity-ci` (live observability via inline `otel-cli` spans, separate from this archive)
