---
id: TASK-2026-05-14-001
title: Nightly workflow failures review and remediation
status: todo
track: investigation
complexity: standard
slice: qa
created: 2026-05-14
owner: product_manager
---

# Nightly Workflow Failures — Summary & Course of Action

## 1. Workflow inventory (scheduled crons, UTC)

| Workflow | Cron | Recent state |
|---|---|---|
| `orchestrator.yaml` (Copa patch nightly) | `0 2 * * *` | **✅ green for 14+ consecutive days** |
| `integer-orchestrator.yaml` | `0 3 * * *` | Mixed — 2/14 success, 2026-05-13/14 green after weeks of failures |
| `chart-gen.yaml` | `0 4 * * *` | ✅ green |
| `chart-integration.yaml` | `0 5 * * *` | **❌ failing every night for 19+ consecutive days** (since at least 2026-04-26) |
| `build-site.yaml` | `0 5 * * *` | Mostly green (runs after others; not gated on chart-integration failure) |

Today 2026-05-14: orchestrator ✅, integer-orchestrator ✅, chart-gen ✅, chart-integration queued, build-site queued.

## 2. Root-cause analysis

### 2a. `chart-integration` — chronic 19+ days red

Failure: `Run chart smoke test` step in **23 of 54 chart jobs** consistently fails. Same 23 charts fail every night
(argo-cd, airflow, cert-manager-csi-driver, cilium, cluster-autoscaler, crossplane, dex, etcd, falco, fluent-bit, gitea,
jenkins, meilisearch, mimir-distributed, nfs-subdir-external-provisioner, opensearch, opensearch-dashboards, openbao,
prometheus, velero, victoria-logs-single, weaviate, workload-identity-webhook).

Common error signature (sampled across 4 different charts):

```
Error: INSTALLATION FAILED: resource Deployment/<chart>/<chart> not ready.
  status: InProgress, message: Available: 0/1
exit status 1
--- FAIL: TestCharts/<chart> (≈600s)
```

`helm install --wait --timeout 10m0s` times out waiting for workloads to become Available in a fresh `kind` cluster on a
GitHub-hosted runner. Helm OCI pull from `ghcr.io/verity-org/charts/...` succeeds (`Pulled:
ghcr.io/verity-org/charts/<chart>:<v>`); pods never reach Ready.

Trend: failing jobs grew from 23 → 34 → 23 across last 5 nightlies, suggesting infra/resource saturation rather than a
single regression.

Tracking issue: [#318](https://github.com/verity-org/verity/issues/318) "Chart-wrapper backlog: verity-rebuild
compatibility long-tail" already documents the long-tail; this dashboard is **stale** for the actual rolling status.

### 2b. `integer-orchestrator` — flaky, recovered

`Dispatch builds` step failed 2026-05-05…05-12 (8 consecutive nights), then green 05-13 + 05-14. Failure was `gh
workflow run` returning `exit 1` inside the per-image dispatch loop — likely API rate-limit or transient
`workflow_dispatch` 4xx; resolved without code change after PRs #335 (wave 3 bespoke) merged.

### 2c. `orchestrator.yaml` — healthy

Copa nightly patch is the workflow that exercises the SCR-2026-05-06-001 change. It has been **green every single
night** since merge, validating the rebased-fork strategy chosen in DISCUSSION-001.

## 3. How we've handled this historically

Searched session memory and `tasks/`:

- **DISCUSSION-001 / SCR-2026-05-06-001** is the only prior nightly-adjacent investigation. It rescued the Copa fork via
  rebase + pseudo-version pin instead of dropping the replace directive. Methodology used: probe → ground in repo →
  propose strawman → delegate per phase → preserve `task_id` for bouncebacks. Reusable pattern.
- No prior task or SCR has formally addressed the chart-integration red dashboard. It has been silently accepted for at
  least 19 days — direct contradiction of `AGENTS.md` ownership mandate: *"never dismiss as pre-existing. If CI is red,
  fix it."*
- Recent commits show **per-chart hot-fixes** ([#329](https://github.com/verity-org/verity/pull/329),
  [#331](https://github.com/verity-org/verity/pull/331), [#332](https://github.com/verity-org/verity/pull/332),
  [#333](https://github.com/verity-org/verity/pull/333), [#334](https://github.com/verity-org/verity/pull/334),
  [#336](https://github.com/verity-org/verity/pull/336)) targeting individual chart probe / value gaps, but no systemic
  remediation.

## 4. Course of action

### Phase 0 — Triage (today, ~30 min, PMA-direct)

1. Confirm whether the 23-chart failure set is **infra-bound** (runner CPU/memory/disk) or **chart-content** by sampling
   3
   jobs' `kubectl describe pod` artifacts.
2. Decide split between (a) raise runner resource class / split jobs across matrix shards, vs (b) per-chart values/probe
   fixes.

### Phase 1 — Issue + SCR (BA + Tech Lead, standard complexity, `qa` slice)

Open SCR `SCR-2026-05-14-001-chart-integration-recovery` capturing:

- AC-1: chart-integration nightly green for 5 consecutive nights.
- AC-2: per-chart failure modes documented in evidence packet.
- AC-3: any chart still red is either fixed or **explicitly disabled** in the matrix with a tracking issue.
- Out of scope: chart feature work; this is recovery only.

### Phase 2 — Remediation (delegated, sequential, atomic per chart)

Process the 23 failing charts in priority groups (critical-path first: argo-cd, prometheus, cilium, falco, opensearch).
For each:

1. Reproduce locally via `go test -tags=integration -run TestCharts/<chart> ./test/chart-integration/...`
2. Capture pod events; classify (resource / probe / image-pull / config).
3. Land minimal fix or matrix-skip with linked issue.
4. Verify next nightly run.

### Phase 3 — Guardrails

1. Add a `chart-integration-summary` job that posts pass/fail counts to a stable status comment on a tracking issue — no
   more silent rot.
2. Promote chart-integration to a **required check** for chart PRs once green for 5 nights.
3. Add `nightly-status` Slack/issue notifier on red so this doesn't repeat.

## 5. Open questions for PO

1. **Scope:** recover all 23 charts in one SCR, or split into critical-path (≤8 charts) vs long-tail?
2. **Priority vs in-flight work:** nothing else is currently active per `tasks/current.md` — proceed immediately?
3. **Budget for matrix-skips:** acceptable to ship Phase 1 with N charts intentionally disabled if they need feature
   work,
   with linked issues? Strawman: yes, max 5.

## 6. Recommended next action

Open the SCR draft and the tracking GH issue today. Wait for PO sign-off on questions 1–3, then begin Phase 2 with the
first critical chart (recommend `argo-cd` — best-documented failure trace, high blast radius).
