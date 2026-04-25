# Verity Metrics Archive

This is an **orphan branch** — separate history from `main`. It is the durable
archive of per-image metrics emitted by Verity's nightly patching pipeline.

**Do not merge this branch into `main`.** It is append-only and consumed by:

- The `verity metrics` Go subcommand (Phase 4) — sparse-checked-out by
  `build-site.yaml` to render the `verity.supply/status` page
- Long-term Patch Lag SLO + CVE Burndown analysis

## Structure

```
README.md             (this file)
_metrics/
├── SCHEMA.md         (data contract, versioned v1)
└── runs/
    └── YYYY-MM-DD/
        └── <run-id>-attempt-<run-attempt>/
            └── <image-safe-name>.json
```

## Producer

- `.github/workflows/patch-image.yaml` finalize step builds
  `metrics-${image}-${tag}.json` per child run
- `.github/workflows/metrics-finalize.yaml` (workflow_run-triggered) downloads
  the artifact and commits it here, with a 5× retry-loop on push conflicts

## Schema

See [`_metrics/SCHEMA.md`](_metrics/SCHEMA.md) for the per-run JSON contract.

## Retention

- Phase 1: no automated pruning; manual operator review at 90 days
- Phase 2 (deferred): quarterly compaction → monthly summaries
