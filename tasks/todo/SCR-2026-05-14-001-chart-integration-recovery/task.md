---
id: TASK-SCR-2026-05-14-001
scr: SCR-2026-05-14-001
title: Chart-integration nightly recovery (one-shot remediation)
status: active
track: implementation
complexity: complex
slice: core
created: 2026-05-14
owner: product_manager
related_issues:
  - "#318"
  - "#324"
  - "#325"
prior_scr: SCR-2026-04-30-001
---

# TASK — Chart-integration nightly recovery

Implementation task for `SCR-2026-05-14-001`. Decomposed into 10 slice-aligned subtasks (see SCR §6).

## Pre-sync Specialist Quorum

Per `docs/core/task_model.md` defaults for `complex` + UI-absent: **business_analyst, technical_architect, tech_lead**. `qa_engineer` joins from subtask 1 onward.

## Subtask Ledger

| # | Slice | Specialist | Title | Status | Evidence |
|---|---|---|---|---|---|
| 1 | qa | qa_engineer | Drill 16 unsampled chart dumps, confirm bucket assignment | pending | `evidences/.../logs/bucket-confirmation.md` |
| 2 | core | developer | Bucket A — `argv[0]` shim symlinks for Cobra-multitool images (argocd, dex, +peers) | pending | image YAML diff + local container smoke |
| 3 | core | developer | Bucket B — Audit `/usr/local/bin/<x>` symlinks across all `images/*.yaml`; fix etcd publication | pending | audit script + image YAML diff |
| 4 | core | developer | Bucket C — `busybox`/`coreutils` packaging or chartValues init-image swap for argocd/repo-server + dex-server | pending | image YAML or chartValues diff |
| 5 | logic | developer | Harness `helm install` retry wrapper for pull-class errors (3×, 30 s backoff) | pending | `test/chart-integration/harness.go` + unit test |
| 6 | logic | developer + tech_lead | `test/chart-integration/SKIPS.yaml` schema + loader + summary report | pending | code + spec doc |
| 7 | core | developer | Per-chart `chartValues` overrides (Bucket E probe tuning + Bucket F falco) | pending | values files |
| 8 | qa | qa_engineer | Trigger `orchestrator` → `chart-gen` → `chart-integration` full pipeline; confirm taxonomy fix rate | pending | run URLs + per-shard outcome |
| 9 | docs | business_analyst + technical_architect | Refresh `ARCHITECTURE.md`, `docs/architecture/TECHNICAL_ARCHITECTURE.md` re SKIPS.yaml + retry wrapper | pending | doc diff |
| 10 | polish | tech_lead | Atomic commit + PR; 5-night green-streak watch; final commit flipping PR-strict mode (AC-8) | pending | PR URL + 5 nightly run URLs |

## Phasing

Per PO decision:
- **Phase 1 PR (subtasks 1–9):** image fixes + harness retry + skip-list + chartValues + docs.
- **Phase 2 PR (subtask 10b only):** flip `continue-on-error: ${{ github.event_name == 'pull_request' }}` to `false` after 5-night green streak.

## Skip-list Budget

Max 5 charts may be marked `expected-skip` in `SKIPS.yaml`. Each requires a linked tracking issue and an exit-criteria comment. Strong candidates today: falco (#325).

## Acceptance Criteria Traceability

ACs 1–9 mirror `docs/scrs/SCR-2026-05-14-001-chart-integration-recovery.md` §2. Each subtask explicitly lists the ACs it advances:

- Subtask 1 → AC-3 input
- Subtask 2 → AC-5
- Subtask 3 → AC-6
- Subtask 4 → AC-7
- Subtask 5 → AC-4
- Subtask 6 → AC-3
- Subtask 7 → AC-1 (E + F)
- Subtask 8 → AC-1 verification
- Subtask 9 → AC-9
- Subtask 10 → AC-2 + AC-8

## Pre-Sync Outcome (2026-05-14, PMA-synthesized — PO approved)

Three amendments folded into SCR §2:
- AC-1 tightened: ≥18 green AND ≤5 skipped (hard cap).
- AC-2 clock: starts at first nightly *after* `orchestrator.yaml` confirms image republish (per subtask 8).
- AC-4 retry scope: pull-class errors only; crash-class fails fast.

Architect note: Bucket A fix vector is **shim symlinks** (chart-version-invariant), not chartValues `command:` overrides. Record rationale in commit message of subtask 2.

Tech Lead note: Subtasks 1 + 5 can run in parallel with subtask 2; subtask 6 depends on subtask 1. Subtask 8 is on the 24 h critical path post-PR-merge; Phase 1 PR does NOT block on subtask 8 completion.

## Pre-Flight (PMA gate before delegation)

- [x] Repo state clean on `opencode/gentle-otter` worktree (tasks/current.md modified only)
- [x] No other shared-worktree implementation task active
- [x] `origin/main` fetched; HEAD is `68d410440a` (chore(deps): astro v6.1.10)
- [x] Today's nightly (run 25848122067) confirmed 23-fail / 31-pass — bucket analysis still applies
- [x] Branch strategy: `fix/chart-integration-recovery` from `origin/main`, rebased before PR

## Reopen History

_(none)_

## Discussion Record

_(none)_

# Post Implementation Task Updates

## qa_engineer: Post Implementation Expectations (subtask 1, 2026-05-14)

### Artifacts produced
- `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/bucket-confirmation.md` — 16 per-chart rows (15 evidence-confirmed; 1 unclassified due to missing artifact) + bucket rollup table covering all 23 chronic charts + subtask-routing input lists.

### Artifacts touched (no edits)
- `images/mimir.yaml`, `images/fluent-bit.yaml`, `images/cluster-autoscaler.yaml`, `images/velero.yaml` — read only, to cross-reference declared `entrypoint:` against the runtime `permission denied` failures.

### Per-bucket summary (16 newly drilled charts)
- **Bucket H (NEW — rebuild exec permission):** 4 — cluster-autoscaler, fluent-bit, mimir-distributed, velero.
- **Bucket B (FHS path / symlink missing):** 4 — gitea, opensearch-dashboards, victoria-logs-single, (+ etcd from prior drill).
- **Bucket E (probe race):** 6 — airflow, cert-manager-csi-driver, meilisearch, openbao, weaviate, workload-identity-webhook.
- **Bucket G (chartValues / env config):** 2 — opensearch (JVM gc.log perm), nfs-subdir-external-provisioner (NFS server unreachable; skip candidate).
- **Unclassified — artifact missing:** 1 — vault (job was green in run 25785628501; intermittent, not chronic).

### Anomalies (full detail in bucket-confirmation.md "Anomalies / risks")
1. **New Bucket H discovered** — systematic apko/melange flaw where a Wolfi-rebuilt binary at the declared `entrypoint:` path lacks execute permission. May also explain etcd's prior Bucket-B "fix not taking effect." Recommend subtask 3 add a `crane export | tar -tv` permission check to its audit script.
2. **gitea init path is `/usr/sbinx/`** (possible chart typo or templating quirk) — subtask 3 should read the pod's actual `containers[].command` rather than infer from logs alone.
3. **openbao test pod returned exit 0 ("sealed: false")** — the chart is fully functional; the failure is purely `helm install --wait` timing. Subtask 5's retry wrapper + a global `--timeout` raise may resolve it without chartValues changes.
4. **vault was green in this run** — cannot be classified from run 25785628501. Defer drill to first failing-vault nightly post-Phase-1; do NOT block subtask 6 SKIPS.yaml on vault.

### Recommended skip-list slots (subtask 6 input, budget = 5)
1. `falco` — Bucket F, [#325](https://github.com/verity-org/verity/issues/325).
2. `nfs-subdir-external-provisioner` — Bucket G (needs NFS server in harness; new tracking issue required).
Three slots held in reserve for {airflow, openbao, opensearch} if their respective fix attempts in subtask 7 don't land.

### Acceptance Criteria coverage
- **AC-3 (per-chart taxonomy → SKIPS.yaml input):** ✅ provided.
- AC-5/6/7 inputs (image-layer fix targets): also provided as part of the rollup, beyond strict subtask 1 scope.

### Constraints honored
- No `images/*.yaml`, `test/chart-integration/`, or workflow files modified — drill-and-classify only.
- All fix-vector proposals use the bucket vocabulary (added Bucket H per task allowance for genuinely-new pattern).

## developer (subtask 5): Post Implementation Expectations
- Files changed:
  - `test/chart-integration/harness_retry.go` (new) — classifier + retry wrapper
  - `test/chart-integration/harness_retry_test.go` (new) — unit tests
  - `.github/workflows/chart-integration.yaml` (one line) — extended `-run` regex on the *Harness compile and unit-test gate* step to include `TestClassifyHelmFailure*`. No other workflow changes.
- New tests added (all under prefix `TestClassifyHelmFailure*`):
  - `TestClassifyHelmFailure` (17 sub-cases covering ErrImagePull, ImagePullBackOff, 502/503/504, DNS lookup failure, TLS handshake timeout, manifest unknown, CrashLoopBackOff, RunContainerError, CreateContainerConfigError, container exit-code=1, init-container crash, exit-code=0 not-a-crash, empty input, generic helm timeout, case-insensitive needles, multi-pod pull, and the crash-vs-pull precedence rule).
  - `TestClassifyHelmFailureStringer` — verifies failure-class log strings (`pull-class`, `crash-class`, `unknown`) used by retry logs.
  - `TestClassifyHelmFailureParsesPodJSON` — exercises the kubectl-JSON projection feeding the classifier.
  - `TestClassifyHelmFailureMalformedJSON` — robustness against partial kubectl output.
- Classifier behavior matrix:
  - pull-class **13 variants** → retry: 10 helm-stderr needles (ErrImagePull, ImagePullBackOff, manifest unknown, failed to pull and unpack, failed to resolve reference, connection refused, i/o timeout, no such host, dial tcp, TLS handshake timeout, 502/503/504), 3 pod waiting reasons (ErrImagePull, ImagePullBackOff, RegistryUnavailable).
  - crash-class **6 variants** → fail-fast: 5 pod waiting reasons (CrashLoopBackOff, RunContainerError, CreateContainerError, CreateContainerConfigError, StartError), plus container lastState.terminated.exitCode≠0 (init or regular).
  - unknown → fail-fast (conservative). Empty input → unknown.
  - **Precedence:** crash-class beats pull-class globally (even across different pods in the namespace) — verified by `TestClassifyHelmFailureParsesPodJSON` mixed-pod case and the mixed-container sub-case in `TestClassifyHelmFailure`.
- Retry config: `MaxAttempts=3`, `Backoff=30s`. Wrapper logs `attempt N/3 failed: classification=<class>, retrying in 30s` / `..., NOT retrying`. Cleanup between attempts reuses existing `UninstallChart` path — no bespoke teardown.
- Local unit test status: **PASS — 21 sub-tests, 0 failures, 0 skips** (evidence: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/subtask-5-unit-tests.log`, exit 0).
- Awaiting subtask 8 to validate against live nightly (the wrapper itself is opt-in via `InstallChartWithRetry` — `main_test.go` still calls plain `InstallChart` so this subtask is a pure addition; subtask 8 / a follow-up can flip the call site once the SKIPS.yaml + chartValues subtasks land and pull-class is the dominant remaining failure mode).
