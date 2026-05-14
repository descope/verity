---
id: SCR-2026-05-14-001
title: Chart-integration nightly recovery (one-shot remediation)
status: approved
type: SCR
track: implementation
complexity: complex
slice: core
created: 2026-05-14
owner: product_manager
related_issues:
  - "#318"  # chart-wrapper backlog
  - "#324"  # probe-race cluster
  - "#325"  # falco kernel-incompat
  - "#308"  # 4-night chronic failure (closed)
  - "#293"  # missing-allowlists root SCR (closed)
prior_scr: SCR-2026-04-30-001  # PR-strict mode deferral
---

# SCR-2026-05-14-001 — Chart-integration nightly recovery

## 1. Context

`chart-integration` nightly has been red **every night for 19+ days** (since at least 2026-04-26). Per-chart hot-fixes (#329 – #336) have reduced failing shards from 40 → 23. The remaining 23 are now in a **chronic 13-of-13 failure bucket** that cannot be resolved one chart at a time without entering a 6-week tail.

The root-cause analysis (see `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/failure-taxonomy.md`) shows the 23 failures collapse to **6 shared buckets**, fixable with 5 cross-cutting patches plus a harness-level "expected-skip" annotation. This is fixable in **one PR**, not 23.

Until this lands, `chart-integration` provides zero PR-gating signal (SCR-2026-04-30-001 deferred strict mode pending exactly this recovery). PRs can merge that break the smoke matrix and nobody will notice.

## 2. Acceptance criteria

- **AC-1** Of the 23 chronic-failing charts in run 25785628501 (2026-05-13), **≥18 are green** in the first qualifying nightly and **≤5 are explicitly annotated `expected-skip`** in `SKIPS.yaml`, each with a linked tracking issue and exit-criteria comment.
- **AC-2** `chart-integration` nightly returns `success` overall (all non-skip shards green) for **5 consecutive scheduled runs**. The clock starts at the **first nightly that runs after `orchestrator.yaml` has republished every image touched by Phase 1** (confirmed by image-tag SHA comparison in subtask 8's evidence).
- **AC-3** Per-chart failure taxonomy from `evidences/.../failure-taxonomy.md` is materialized as `test/chart-integration/SKIPS.yaml` (or equivalent) with one entry per skip + linked issue.
- **AC-4** Harness gains a retry-on-pull-error wrapper that retries `helm install` up to 3 times when the failure is `ErrImagePull`/`ImagePullBackOff`/`502 Bad Gateway`/network-class. **Crash-class errors (`CrashLoopBackOff`, container exit ≠ 0 after start) are NOT retried** — they fail fast.
- **AC-5** Bucket A (Cobra entrypoint) is fixed at the image layer for at least argo-cd + dex (proof: container starts without "unknown command" error on local kind run).
- **AC-6** Bucket B (FHS path) is fixed for etcd — confirm published image has working `/usr/local/bin/etcd` symlink.
- **AC-7** Bucket C (missing sh/cp) is fixed for argo-cd repo-server + dex-server initContainers, either via image patch or chartValues override.
- **AC-8** SCR-2026-04-30-001 PR-strict mode is **flipped on for PRs** as the final commit of this SCR (chart-integration becomes a required check).
- **AC-9** `docs/architecture/TECHNICAL_ARCHITECTURE.md` and `ARCHITECTURE.md` updated to describe the `SKIPS.yaml` mechanism and the retry wrapper.

## 3. Scope

### In scope
- Image-build YAML changes under `images/` for buckets A/B/C affected images.
- Harness changes in `test/chart-integration/` (retry wrapper, skip-list loader).
- Per-chart `chartValues` overrides where image rebuild is wrong tool (probe tuning, falco ebpf mode, init-image swap).
- One trigger run of `orchestrator.yaml` post-merge to refresh affected published image tags.
- Documentation refresh.
- Flipping `continue-on-error` to `false` on PRs.

### Out of scope
- Bucket D root cause investigation (GHCR 502s) — masked by retry wrapper. Follow-up SCR if rate persists.
- Bucket F (falco kernel-incompat) fundamental fix — landed as `expected-skip` with [#325](https://github.com/verity-org/verity/issues/325) follow-up.
- New chart additions to the matrix.
- Wolfi-rebuild architectural changes (multi-arch, FIPS variants).
- Chart version bumps (only image content fixes).

## 4. Risk surface

- Image patches may need fresh `orchestrator.yaml` run to publish. Sequencing matters: image push → wait for `Chart Generation` → wait for next chart-integration nightly.
- `expected-skip` mechanism is new code; harness must fail closed if `SKIPS.yaml` is malformed.
- Flipping strict mode on PRs (AC-8) will block any PR whose change touches a still-broken chart. Mitigation: land AC-8 in a separate atomic commit after 5-night green streak proves stability.

## 5. Validation plan

| Phase | Step | Output |
|---|---|---|
| P1 | Drill remaining 16 unsampled chart dumps to confirm bucket assignment | `evidences/.../logs/bucket-confirmation.md` |
| P2 | Apply Bucket A/B/C image patches on a feature branch | image YAML diff |
| P3 | Manually dispatch `orchestrator.yaml` against the ~10 affected images | run URL + image SHAs |
| P4 | After Chart Generation completes, manually dispatch `chart-integration` | shard pass/fail counts |
| P5 | Iterate per-chart chartValues for Bucket E charts | per-chart values files |
| P6 | Land `SKIPS.yaml` + harness changes | code review |
| P7 | Watch 5 nightlies; address regressions | nightly run links |
| P8 | Final commit: flip PR-strict mode | PR linked to this SCR |

## 6. Decomposition (slice-based subtasks)

This SCR is `complex`/`implementation`, expected to be delegated via `workflow_runner` once approved. Slice breakdown:

| Subtask | Slice | Specialist | Notes |
|---|---|---|---|
| 1. Bucket confirmation drill (16 charts) | `qa` | qa_engineer | depends only on artifact downloads |
| 2. `images/argocd.yaml` + `images/dex.yaml` + Bucket A peers entrypoint shim | `core` | developer | image YAML patches |
| 3. `images/etcd.yaml` (and audit) FHS symlinks complete | `core` | developer | 30-min audit |
| 4. Bucket C init-tool packaging or chartValues override | `core` | developer | argocd, possibly jenkins/gitea |
| 5. Harness `helm install` retry wrapper | `logic` | developer | `test/chart-integration/harness.go` |
| 6. `test/chart-integration/SKIPS.yaml` + loader + report | `logic` | developer + tech_lead | covers F + tail |
| 7. Per-chart `chartValues` overrides for E + F | `core` | developer | values files under existing convention |
| 8. Trigger orchestrator → chart-gen → chart-integration full pipeline | `qa` | qa_engineer | proof-of-life |
| 9. Documentation refresh | `docs` | business_analyst + technical_architect | ARCHITECTURE.md + SCR closure |
| 10. Atomic commit + PR + 5-night watch + strict-mode flip | `polish` | tech_lead | merges in 2 commits separated by 5-night window |

## 7. Open questions for PO

1. **Strict-mode flip timing (AC-8):** flip in same PR as the fix (riskier, faster), or 5 nights later as a follow-up commit (slower, safer)? **Strawman: 5 nights later.**
2. **Skip-list budget:** how many charts may be marked `expected-skip`? **Strawman: max 5, each with a linked tracking issue.**
3. **Bucket D (GHCR 502):** retry-only or also open infra ticket? **Strawman: retry-only now, track 502 rate via a one-line metric in the harness, escalate if >0.5% over 30 days.**
4. **Single PR vs phased PRs:** Phase 1 (image fixes + retry + skip-list) lands first as one PR; strict-mode flip as a tiny second PR after the 5-night green streak. Acceptable?

## 8. Next action

Awaiting PO approval. Once approved, PMA will:
1. Promote `tasks/todo/SCR-2026-05-14-001-chart-integration-recovery/task.md` to active.
2. Initiate `workflow_runner` for complex implementation track.
3. Pre-sync: BA + Technical Architect + Tech Lead.
