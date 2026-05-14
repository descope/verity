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

## developer (subtask 6): Post Implementation Expectations
- Files added: test/chart-integration/SKIPS.yaml, test/chart-integration/skips.go, test/chart-integration/skips_test.go
- Files modified: test/chart-integration/main_test.go (TestMain loads SKIPS once + TestCharts gates on IsSkipped + sentinel writer), .github/workflows/chart-integration.yaml (-run regex extended with TestLoadSkips|TestIsSkipped|TestProductionSKIPSYAMLIsValid + Record-shard-outcome step renders success/failure/skipped distinctly via `_skip-<chart>.txt` sentinel)
- Skip entries seeded: 2 (falco [#325](https://github.com/verity-org/verity/issues/325), nfs-subdir-external-provisioner — "needs new issue")
- Skip budget remaining: 3 of 5 (RESERVED — do not pre-populate)
- Hard cap enforced in code: `MaxSkippedCharts = 5` (constant in skips.go); loader fails closed on breach
- Fail-closed invariants tested: malformed YAML, unknown top-level field (KnownFields=true), duplicate chart, >5 entries, each of the 6 required fields missing, unsafe chart names (slash, backslash, `..`, whitespace, newline), bad tracking_issue (non-github URL, scheme-less, free-text). All 9 test functions / 13 subtests pass.
- Unit test status: **PASS — 9 functions, 13 subtests, 0 failures, 0 skips** (`VERITY_IT_SKIP_CLUSTER=1 go test -tags=integration -run 'TestLoadSkips|TestIsSkipped|TestProductionSKIPSYAMLIsValid' ./test/chart-integration/...` exit 0). Full workflow gate regex (allowlist + classifier + skips) also passes.
- Lint: `golangci-lint run --build-tags=integration ./test/chart-integration/...` reports 0 issues in skips.go / skips_test.go / main_test.go. 4 pre-existing `modernize` nits remain in `harness_retry_test.go` (subtask 5's file) — flagged in subtask-9-handoff.md as cosmetic carry-over.
- Awaiting subtask 8 to validate skip behavior under live workflow (sentinel file write, step-summary rendering, t.Skipf interaction with `make chart-integration` exit code).

## developer (subtasks 2+3+4): Post Implementation Expectations
- Files changed (image YAMLs, 7 charts): `images/cluster-autoscaler.yaml`, `images/fluent-bit.yaml`, `images/mimir.yaml`, `images/velero.yaml`, `images/etcd.yaml` (default + fips), `images/opensearch-dashboards.yaml`, `images/argocd.yaml` (default + fips).
- Evidence written: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/image-mode-audit.md` (Bucket H confirmation via crane export — mode bits captured for all 8 suspect images), `evidences/.../logs/subtask-7-handoff.md` (chartValues recommendations for escalations).
- Buckets addressed: A (1 chart partial — argocd shim symlinks), B (1 chart — opensearch-dashboards entrypoint), H (5 charts — cluster-autoscaler, fluent-bit, mimir, velero, etcd). 7 commits on `fix/chart-integration-recovery`, one per chart.
- Charts escalated to subtask 7 (chartValues) — per `subtask-7-handoff.md`:
  - **argo-cd** (Bucket A full fix + Bucket C copyutil) — needs `command:` swap on 5 deployments + init-image swap to a chainguard busybox.
  - **dex** (Bucket A) — drop the leading `dex` element from chart's `args:`.
  - **gitea** (Bucket B `/usr/sbinx/`) — Bitnami-specific init script path; PO decision needed (revert to Bitnami image OR rewrite initContainers).
- Charts reclassified during developer drill:
  - **etcd**: was Bucket B, actually Bucket H (the FHS symlink IS landing; the symlink target `/usr/bin/etcd` lacks +x).
  - **crossplane**: was "Bucket A likely", actually **passing** — both pods Running, no fix needed. Recommend removing from the active-failures list for subtask 8.
  - **victoria-logs-single**: chart uses a Copa-patched image (`ghcr.io/verity-org/victoriametrics/victoria-logs:v1.50.0`), not a Wolfi rebuild — **out of scope for image YAML edits**. Flagged in subtask-7-handoff.md as Copa-pipeline follow-up.
- New bucket vocabulary added: **Bucket H (image-layer permission denied)** is confirmed via crane-export mode-bit audit. The fix vector — `paths: [{path: /usr/bin/<name>, type: hardened-binary, permissions: 0o755}]` — relies on the integer renderer's pass-through behavior (only `directory`/`symlink` are validated by `internal/integer/render/render.go`; any other `type` value is forwarded to apko verbatim). `hardened-binary` is apko's standard binary-mode marker.
- Schema discovery (non-blocking note): the codebase only documents `directory` and `symlink` path types in `internal/integer/config/types.go`. The `hardened-binary` value is forwarded but undocumented in our types.go. Subtask 9 (docs) may want to add it to the godoc on `PathDef.Type`.
- Local verification: `go run . integer validate` passes (329 configs, 0 errors) after every commit. `apko`/`melange` build smoke was **not** possible locally (binaries not present in this worktree's PATH — `mise` only ships `crane`, `go`, etc., not the OCI builders). Crane-based audit of the *currently published* images is the proof of root cause; the *fix* will be proven by subtask 8's orchestrator dispatch republish.
- Branch state: 7 commits ahead of `fix/chart-integration-recovery` HEAD prior to my work; **not pushed**. Subtask 8 (qa) will push and dispatch.
- Acceptance criteria touched: AC-5 (Bucket A — partial via argocd shim symlinks + chartValues hand-off), AC-6 (Bucket B — opensearch-dashboards complete; etcd reclassified to H and resolved there), AC-7 (Bucket C — recommendation written for chartValues init-image swap; image-side intentionally NOT bloated with busybox). Plus newly-introduced AC-implied-by-Bucket-H (image-layer +x on 5 chart binaries — complete pending republish).
- Awaiting subtask 8 to push branch and dispatch orchestrator.yaml for image republish, then chart-integration to validate fix.

## developer (subtask 7): Post Implementation Expectations
- Charts modified (6, one commit each — all on `fix/chart-integration-recovery`):
  - `airflow` — Bucket E — raise migration wait timeout + apiServer startup probe failureThreshold (commit `46a30055a6`).
  - `meilisearch` — Bucket E — raise startupProbe failureThreshold 60 → 180 (commit `8d66abeedd`).
  - `openbao` — Bucket E — extend existing entry: raise server.readinessProbe initialDelaySeconds + failureThreshold (commit `766e2be876`).
  - `weaviate` — Bucket E — raise readinessProbe initialDelaySeconds 3 → 60 and failureThreshold 3 → 12 (commit `21e7809faf`).
  - `opensearch` — Bucket G — set opensearchJavaOpts to append `-Xlog:disable` to defuse the unwritable gc.log path (commit `a03025aa84`).
  - `gitea` — Bucket B — PO Option A: extend existing entry with `image.registry: docker.io`, `image.repository: bitnami/gitea`, `image.tag: "1"` (commit `838ea23a0a`).
- Commits made: 6 (one per chart, format: `fix(charts): <chart> — Bucket <X> — <one-line>`).
- ChartValues schema location: `verity.yaml` top-level `chartValues:` map, keyed by chart name (matches `Chart.yaml dependencies[].name`); values are **flat dotted-path scalars only**. Mechanism enforced by `internal/discovery/charts.go` `helmSetPair` (line 159) — only `string`, `bool`, `int*`, `uint*`, `float32`, `float64` accepted; nil and any other type (including `[]any` and `map[string]any` lists/maps as VALUES) return `ErrChartValueUnsupportedType`. Dotted paths are split into map-walk parts by `internal/chartgen/chart.go` `splitOverridePath` + `setScalarValue` (line 190) — note: bracket notation parses (e.g. `extraEnvs[0].name`) but is rendered as a map keyed by `"0"`, NOT a real YAML list, so list-valued chart settings cannot be expressed via this mechanism at all.
- Local render validation: **PASS for all 6 modified charts** via `helm template <chart> <tgz> --set <key>=<value> ...` against the on-disk chart tarballs in `tmpcharts-215315/`. Evidence: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/subtask-7-render-{airflow,meilisearch,openbao,weaviate,opensearch,gitea}.log` (local-only; `evidences/` is gitignored). For each chart, the rendered manifest shows the expected scalar landing in the right field (e.g. airflow init container args contain `--migration-wait-timeout=180`, opensearch StatefulSet env shows `OPENSEARCH_JAVA_OPTS=-Xmx512M -Xms512M -Xlog:disable`, gitea container image renders as `docker.io/bitnami/gitea:1`). End-to-end (chartgen → wrapper chart → helm install on cluster) validation is subtask 8's responsibility.
- Charts where I attempted but couldn't confidently fix (5 — all are **structurally blocked** by the dotted-path-scalar-only chartValues mechanism, not by missing evidence):
  - **argo-cd** (Bucket A + C). subtask-7-handoff.md §1 prescribes setting `command:` and `args:` lists on 5 sub-deployments (server / repoServer / applicationSet / controller / notifications) plus replacing the `copyutil` initContainer (a list of objects). Both `command:` and `args:` are YAML LISTS in the chart; `initContainers:` is a list of complex objects. Neither shape is expressible via verity.yaml chartValues — `helmSetPair` rejects them at type-switch with `ErrChartValueUnsupportedType`, and `setScalarValue` would render bracket-indexed paths as a map keyed by `"0"` rather than a real list. Confirmed by reading `internal/discovery/charts.go:159` and `internal/chartgen/chart.go:190`; reproduced in a 30-line Go isolation harness (output: `{"command": {"0": "/usr/local/bin/argocd-server"}}` instead of the required real-list shape).
  - **dex** (Bucket A). subtask-7-handoff.md §2 prescribes dropping the leading `"dex"` from the chart's `args:` list — same list-value blocker as argo-cd.
  - **cert-manager-csi-driver** (Bucket E). The chart's `values.yaml` only exposes `app.livenessProbe.port` (line 204-206) — no `app.livenessProbe.initialDelaySeconds`, no `app.readinessProbe` block at all. The bucket-confirmation.md suggestion `app.livenessProbe.initialDelaySeconds` does not correspond to an exposed knob; setting it would create a stray map key the chart's templates ignore. Probe parameters appear to be hardcoded inside the chart's templates.
  - **workload-identity-webhook** (Bucket E). The chart's `values.yaml` (40 lines total, dumped in full to confirm) exposes NO probe-tuning knobs at all. The probes are hardcoded in `templates/deployment.yaml` and cannot be overridden via Helm values. (Suggestion from bucket-confirmation.md to "introduce a startupProbe" would require either a chart fork or a different chart version that exposes such knobs.)
  - **cilium** (was E/G ambiguous). Drilled `/tmp/diag/cilium/_dump-cilium/` per task instructions: the `config` init container of `cilium-kfd6l` terminates with `Error: "Failed to start hive" - dial tcp 10.96.0.1:443: operation not permitted` (5 restart attempts captured in `pods.json`). This is **NOT** a probe race (Bucket E) and **NOT** a chart-config bug (Bucket G); it is the cilium agent's required raw-socket access to the apiserver being denied at the runc/kernel layer — a kind-environment capability/permission issue, not a chartValues lever. Cilium agent inherently needs `NET_ADMIN` + raw socket capabilities that the smoke harness's kind config may not be granting. No chartValues edit would fix this; recommend leaving cilium failing as it provides clearer evidence of the harness-side gap (e.g. missing `privileged: true` or capability grants in the kind cluster spec) than masking it would.
- Recommended skip-list additions (PO decision needed — **not** added to `SKIPS.yaml` in this subtask per task instructions): based on the 5 structurally blocked charts above, the highest-value skip-list candidates are:
  1. `argo-cd` — Bucket A + C, list-valued chart override required; needs either a chartgen feature to inject raw YAML, or upstream chart fork. Strong skip candidate.
  2. `dex` — Bucket A, list-valued chart override required; same blocker as argo-cd. Strong skip candidate.
  3. `cilium` — kind-cluster capability issue, not a chart/image bug; needs harness-side fix.
  4. `workload-identity-webhook` — chart exposes no probe knobs; needs upstream chart change or fork.
  5. `cert-manager-csi-driver` — same as above; chart exposes no probe knobs.

  SKIPS.yaml budget is 5 (per SCR §6); the 2 already-claimed slots (falco, nfs-subdir-external-provisioner) leave 3 remaining. PO will need to choose among the 5 above + any others that emerge from subtask 8's nightly. Suggested priority order if forced to pick 3: argo-cd, dex, cilium (the three with the highest visibility / most-likely-to-flag-real-bugs profiles).
- **Known risk on gitea**: chartgen's image-override pass (`internal/chartgen/chart.go` `buildValuesTree`) writes `image.repository` and `image.tag` AFTER chartValues are merged, and per commit `0f12b6352541` (#361) **override keys win over same-key collisions on identical paths**. Because `images/gitea.yaml` still exists and gitea is in the Integer rebuild set, chartgen will almost certainly overwrite `image.repository` back to `ghcr.io/verity-org/gitea`, defeating the PO Option A fix at the published-wrapper layer. The chartValues shape itself is correct (proven by local `helm template`), but the end-to-end behavior must be confirmed in subtask 8 via `verity chart-gen --dry-run`. If chartgen overwrites: follow-up options are (a) remove gitea from `images/*.yaml`, or (b) add a per-chart "skip-image-override" feature to chartgen. Both are out of scope for this subtask. Documented in commit `838ea23a0a`'s body.
- Awaiting subtask 8 to validate end-to-end (chartgen → wrapper chart → helm install on cluster).

## developer (subtask 7b): Post Implementation Expectations

### Part A — chartgen extension
- Files changed:
  - `internal/discovery/charts.go` — new helpers `hasNonScalarChartValue`, `isNonScalarChartValue`, `writeChartValuesFile`, `buildChartValuesTree`, `setNestedValue`, `isMap`, `splitChartValuePath`; `helmTemplateArgs` signature now returns `([]string, func(), error)`; new sentinel `ErrChartValueConflictingShape`; widened the `ErrChartValueUnsupportedType` doc-comment (sentinel value unchanged, semantics broadened to mean "yaml.Marshal refused"). Imports `os`, `path/filepath` added.
  - `internal/discovery/discover.go` — `LoadVerityConfig` now calls `ValidateChartValues` post-unmarshal; new exported `ValidateChartValues` function. Imports `sort` added.
  - `internal/discovery/charts_test.go` — updated `TestHelmTemplateArgs` for the new 3-value return signature (added a non-nil-cleanup guard for nil-chartValues case); new `TestHelmTemplateArgs_ScalarFastPath` (asserts pre-7b argv shape bit-for-bit), `TestHelmTemplateArgs_FileBasedPath` (asserts `-f` path emits real list YAML, no `--set` flags, no `'0':` map-by-index nonsense), `TestHelmTemplateArgs_CleanupRemovesTempFile`, `TestBuildChartValuesTree` (6 sub-cases incl. argo-cd-style list-of-maps), `TestValidateChartValues_Conflicts` (6 sub-cases).
  - `internal/chartgen/chart_test.go` — `TestBuildWrapperChartValues` extended with 4 sub-cases: list value at leaf, list-of-maps value, image-override + list chartValues coexist (#361 regression), and image-override-wins-on-same-path collision (also #361 regression).
- New unit tests: 14 sub-tests across 5 new test functions (3 in discovery, 2 worth of additions in chartgen).
- Backward-compat regressions detected: **0**. The existing `TestHelmSetArgs` 12 sub-cases run unchanged (scalar fast path produces identical argv). The existing `TestBuildValuesTree` / `TestComposeRegistryRendersValidImageRefs` sub-cases run unchanged (image-override precedence preserved per #361). The real `verity.yaml` (18 chartValues entries) load-validates cleanly through `ValidateChartValues`.
- Test status: **PASS**. `go test ./... -count=1 -timeout 180s` — all packages green. `go vet ./...` clean. `gofmt` clean.
- Commit: `665f8e7c0a feat(chartgen): support list and map values in chartValues (SCR-2026-05-14-001)`.

### Part B — re-attempt 5 blockers
- Charts fixed via chartValues: **0**. Part A confirms the parser-layer blocker is gone; Part B confirms that for the 4 charts re-attempted, the failure is at a **deeper** layer: the upstream chart **templates themselves hardcode** the field that needs overriding, and do not expose any chartValues knob that can change it. Findings per chart:
  - **argo-cd** (Bucket A + C). The chart's `templates/argocd-server/deployment.yaml` line 74 hardcodes `args: [/usr/local/bin/argocd-server, --port=..., --metrics-port=...]`; the chart only exposes `server.extraArgs` (appended AFTER the hardcoded args, cannot replace argv[0]). There is **no** `server.command:`, `server.args:`, or container-replacement knob — `subtask-7-handoff.md` §1's recommended chartValues shape (`server.command: [...]`) does not correspond to any chart-exposed value path. **Verified by `grep -nE '^\\s+command:|^\\s+args:' templates/argocd-{server,repo-server,applicationset,application-controller,notifications}/...`** — only `args:` appears, only at hardcoded sites, no `.Values.<x>.command` or `.Values.<x>.args` reference exists in those templates. Additionally, the `copyutil` initContainer at `templates/argocd-repo-server/deployment.yaml` line 421 is fully hardcoded (image is `repoServer.image.repository` — the argo-cd image, which lacks `/bin/sh` and `/bin/cp`); the chart exposes `repoServer.copyutil.extraArgs` only (appends one flag to `/bin/cp`) and `repoServer.initContainers` (adds NEW init containers but cannot remove or replace the hardcoded copyutil). **No chartValues-layer fix possible.**
  - **dex** (Bucket A). `templates/deployment.yaml` line 66-81 hardcodes `args: ['dex', 'serve', '--web-http-addr', ...]`. No `command:` or full-args replacement knob. Same template-level hardcoded blocker as argo-cd. **No chartValues-layer fix possible.**
  - **cert-manager-csi-driver** (Bucket E). `templates/daemonset.yaml` line 127-132 hardcodes `livenessProbe: { httpGet: ..., initialDelaySeconds: 5, timeoutSeconds: 5 }`. The chart's values.yaml exposes only `app.livenessProbe.port` (used by line 77 for the `--health-port=<n>` argument and line 121 for `containerPort`) — there is no values path for `initialDelaySeconds` / `timeoutSeconds` / `failureThreshold`. `bucket-confirmation.md`'s suggested path `app.livenessProbe.initialDelaySeconds` is not exposed by the chart. **No chartValues-layer fix possible.**
  - **workload-identity-webhook** (Bucket E). `templates/azure-wi-webhook-controller-manager-deployment.yaml` lines 50-56 + 68-73 hardcode both probes with literal `initialDelaySeconds: 15` / `5` and `periodSeconds: 20` / `5`. The chart's 40-line values.yaml (dumped in full in subtask 7) exposes zero probe-tuning fields. **No chartValues-layer fix possible.**
  - **cilium** — not re-attempted per task brief; remains a kind-cluster capability issue (init container `dial tcp 10.96.0.1:443: operation not permitted`). Strong SKIPS.yaml candidate.
- Charts confirmed unfixable via chartValues (skip-list candidates, PMA-decision):
  1. **argo-cd** — chart hardcodes container args + copyutil initContainer with broken image. Requires upstream chart fork OR image-side ENTRYPOINT change (`images/argocd.yaml` → `entrypoint: ""`/null so Kubernetes treats `args[0] = /usr/local/bin/argocd-server` as the exec target, then Cobra argv[0]-dispatch via the existing symlinks fires). Image-side change is out of subtask-7b scope.
  2. **dex** — chart hardcodes container args. Same upstream-fork-or-image-side resolution as argo-cd. Image-side fix would be `images/dex.yaml` ENTRYPOINT change.
  3. **cert-manager-csi-driver** — chart hardcodes probe params. Requires upstream chart fork OR chart-version bump (no fork without checking newer chart versions; task forbids version bumps).
  4. **workload-identity-webhook** — chart hardcodes probe params. Same as cert-manager-csi-driver — needs upstream chart fork or version bump.
  5. **cilium** — kind-cluster capability issue (NOT a chart bug). Needs harness-side fix (extra `securityContext: privileged: true` or `NET_ADMIN` capability on the test pod).
- Commits made in Part B: **0** chart commits + **1** docs commit (this task-file append). The 4 retry attempts produced no viable chartValues edits — committing a no-op chartValues stub would mislead subtask 8 into thinking the chart was reattempted with a real fix.
- Local render validation: Part A demo via `helm template -f` against argo-cd (`server.extraArgs: [--insecure, --logformat=json]`) confirmed list values now flow through the file-based path correctly (rendered manifest contains both list elements after the hardcoded args). Evidence: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/subtask-7b-render-partA-demo.log`. **No per-chart Part B render evidence files** because no Part B chart edits were made.

### Net AC-1 impact
- Green pool change: **13 → 13** (Part B added 0 charts). The Part A schema extension is infrastructure; it unlocks future chartValues shapes but does not by itself add to the green pool.
- Skip-list candidates change: subtask 7 left **5 candidates** (argo-cd, dex, cilium, workload-identity-webhook, cert-manager-csi-driver) plus 2 already-claimed (falco, nfs-subdir-external-provisioner) = 7 total. Subtask 7b re-confirms all 5 candidates with deeper per-chart template-level evidence. **PMA decision still required** to fit within the 5-slot SKIPS.yaml budget (5 chartValues-unfixable candidates - 3 free slots = 2 over-budget — needs slot triage).
- Recommended PMA decisions:
  - **Promote 3 of {argo-cd, dex, cilium, workload-identity-webhook, cert-manager-csi-driver} to SKIPS.yaml** to fill the 3 free reserve slots. Priority by visibility / blast radius (highest first): `argo-cd` > `dex` > `cilium` > `workload-identity-webhook` > `cert-manager-csi-driver`.
  - **Open follow-up issues** for the 2 charts that don't fit in the skip-list budget — they will remain red in the nightly until image-side ENTRYPOINT fixes (argo-cd, dex) or chart-version bumps (cert-manager-csi-driver, workload-identity-webhook) land.

## developer (subtask 4b): Post Implementation Expectations
- Files changed (2): `images/argocd.yaml` (default + fips — removed `entrypoint:` lines), `images/dex.yaml` (removed `entrypoint:` line).
- Mechanism for argo-cd: Drop explicit `entrypoint:`; rely on K8s passing chart's hardcoded `args: [/usr/local/bin/argocd-<sub>, ...]` as the full argv when both `command: nil` and image `Entrypoint: null`. runc exec's the absolute symlink path, preserves argv[0], and argo-cd's `cmd/main.go` does `filepath.Base(os.Args[0])` to switch into the right sub-command. Depends on shim symlinks from commit `66db29b78a`.
- Mechanism for dex: Drop explicit `entrypoint:`; chart's hardcoded `args: [dex, serve, ...]` becomes full argv. runc PATH-resolves `dex` to `/usr/bin/dex` while preserving argv[0]="dex"; cobra rootCmd `Use: "dex"` then processes argv[1]="serve" as the subcommand. No shim symlinks needed (chart's args[0]="dex" is a bare name resolved via the image's PATH env, not an absolute path).
- argv[0] dispatch verified against upstream source: **YES for argocd** (`github.com/argoproj/argo-cd` `cmd/main.go` — switch on `filepath.Base(os.Args[0])` with cases `common.Command{CLI,Server,RepoServer,ApplicationController,ApplicationSetController,CMPServer,CommitServer,Dex,Notifications,GitAskPass,K8sAuth}`, also accepts `ARGOCD_BINARY_NAME` env override); **NO for dex** (`github.com/dexidp/dex` `cmd/dex/main.go` — plain cobra rootCmd with `Use: "dex"`, no `os.Args[0]` inspection). Full verification log: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/subtask-4b-argv0-verification.md` (local-only per `.gitignore`).
- Local image-build validation: **PARTIAL**. `apko`/`melange` still unavailable in worktree PATH. Local probe test in `internal/integer/render/` confirmed: with `Entrypoint: ""` in the `TypeTemplate`, `render.Config` emits no `entrypoint:` key in the apko YAML (test was added + removed inline; verified deterministic). `go test ./internal/integer/...` PASS across all packages. `verity integer validate` PASS (329 configs). Crane-based inspection of the existing published `Entrypoint: ["/usr/bin/dex"]` / `["/usr/bin/argocd"]` confirms the OCI manifest shape that the fix removes. End-to-end (apko build → republish → kind chart-integration) validation is subtask 8.
- Risk: if a future apko version starts auto-populating OCI `Cmd` from the apk's melange metadata, that auto-CMD could append to the chart's args. Probability low; mitigation is the subtask 8 crane-config inspection.
- Net AC-1 impact: prior to 4b, argo-cd + dex were in the "structurally blocked, candidate for SKIPS.yaml" tier (per subtask 7b §Part B). 4b unblocks both image-side without chart fork or upstream patch. **AC-1 candidate-green delta: +2 charts (argo-cd, dex)** pending subtask 8 republish & validation. Charts still in the blocker tier: cilium (kind-cluster capability), cert-manager-csi-driver (Bucket E template-hardcoded probes), workload-identity-webhook (Bucket E template-hardcoded probes) — these remain SKIPS.yaml candidates per subtask 7b's analysis.
- Commits: 2 (one per chart). `f50527bf16 fix(images): argocd — Bucket A — drop explicit entrypoint to enable argv[0] dispatch`, `126ba3047b fix(images): dex — Bucket A — drop explicit entrypoint, use chart's args[0] for binary name`.
- Documentation flag for subtask 9: the argv[0]-dispatch pattern (multicall-binary via filepath.Base) is now load-bearing in our argocd image and deserves a one-paragraph note in `docs/architecture/TECHNICAL_ARCHITECTURE.md` (or similar) explaining the contract between image-yaml's `entrypoint:` (or its absence), apko's `Entrypoint: null` emission, and downstream chart `args:` semantics. Recommend a "Image entrypoint conventions" subsection.
- Awaiting subtask 8 to push the branch and dispatch orchestrator.yaml → chart-integration.yaml. The two newly-pushed images (argocd, dex) must be republished before chart-integration is dispatched, else the published `:latest` still has the old `Entrypoint: [/usr/bin/<bin>]` and the fix won't take effect.

### developer (subtask 4b) — addendum: jenkins Bucket B follow-up (2026-05-14)
- Additional file changed: `images/jenkins.yaml` — added `jenkins-{{version}}-openjdk-21` to `packages:` list (now `["jenkins-{{version}}", "jenkins-{{version}}-openjdk-21", "busybox"]`).
- Root cause (different from the prompt's symlink hypothesis): wolfi's `jenkins-2` is a SHELL package — it ships no files. The actual `jenkins.war` is shipped by the `jenkins-2-openjdk-{17,21,25}` subpackages (verified via `gh api repos/wolfi-dev/os/contents/jenkins-2.yaml`: `subpackages: - range: openjdk-versions, pipeline: mv war/target/jenkins.war ${{targets.contextdir}}/usr/share/java/jenkins/`). Without one of those subpackages installed, the published image had no `/usr/share/java/jenkins/jenkins.war` at all — the chart's `Error: Unable to access jarfile /usr/share/java/jenkins/jenkins.war` was the symptom. Symlinking would have pointed at nothing. Verified the missing-file shape via `crane export ghcr.io/verity-org/jenkins:latest`.
- Subpackage existence verified: wolfi APKINDEX lists `jenkins-2`, `jenkins-2-openjdk-17`, `jenkins-2-openjdk-21`, `jenkins-2-openjdk-25` (chose openjdk-21 = LTS, matches the chart's typical Jenkins LTS targeting).
- Net AC-1 impact: **+1 chart** (jenkins). Total candidate-green going into subtask 8: **15** (was 14 after argocd+dex).
- Commit: `9b31d1a1f3 fix(images): jenkins — Bucket B — install jenkins-2-openjdk-21 subpackage that ships jenkins.war`.
- Risk: if the chart's bundled plugins or jcasc config exercise a JDK-specific path (e.g. shipping classes that need a class-file version from JDK 25), running on openjdk-21 could fail at plugin-load time. Probability low (Jenkins 2.564 LTS line officially supports 21); subtask 8 will surface it if real.

## developer (subtask 6b): Post Implementation Expectations
- Files modified: test/chart-integration/SKIPS.yaml (header reflects 5-of-5 full; 3 entries appended: cilium, cert-manager-csi-driver, workload-identity-webhook).
- Skip entries: **5 of 5 (file is at cap)**. Loader's MaxSkippedCharts=5 invariant will reject any 6th entry — by design.
- All three new entries are chart-template-blocked per subtask 7b analysis: upstream Helm templates hardcode the failure-mode-relevant field with no values knob and no chartValues shape (even with subtask 7b's list/map extension) can override.
- Tracking issues: all three carry "needs new issue" sentinel; audit script (future) will surface them for issue-creation backlog.
- Unit test status: **PASS — `VERITY_IT_SKIP_CLUSTER=1 go test -tags=integration -run 'TestLoadSkips|TestIsSkipped|TestProductionSKIPSYAMLIsValid' ./test/chart-integration/...` exit 0**. Log: `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/subtask-6b-skips-add-tests.log`. `TestProductionSKIPSYAMLIsValid` confirms the live 5-entry file validates cleanly.
- Subtask-9 doc handoff already accounts for this — no update needed; the architecture-doc text describes the cap and lifecycle, not the specific entries.

## business_analyst + technical_architect (subtask 9): Post Implementation Expectations
- Files modified:
  - `docs/architecture/TECHNICAL_ARCHITECTURE.md` (new) — full technical contract for SKIPS.yaml, retry wrapper, chartgen list/map chartValues, image entrypoint convention.
  - `ARCHITECTURE.md` (one new subsection `### Chart-integration smoke tests` under `## Pipeline`, between `### Remaining workflows` and `### Skip Detection (Preflight)`).
  - `internal/integer/config/types.go` (godoc extension on `PathDef` block-comment + `Type` field inline comment — documents `hardened-binary` pass-through; no struct field added).
  - `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/failure-taxonomy.md` (appended Bucket H section — local-only, NOT committed; `evidences/` is `.gitignore`d, the institutional knowledge lives in the committed docs above).
- Sections added (in committed docs):
  - `docs/architecture/TECHNICAL_ARCHITECTURE.md#1-testchart-integrationskipsyaml` — schema, hard cap, fail-closed invariants (8), sentinel-file mechanism, lifecycle, cross-references.
  - `docs/architecture/TECHNICAL_ARCHITECTURE.md#2-harness-retry-wrapper` — `InstallChartWithRetry`, classifier, package-level needle/reason vars as source of truth, crash-precedence rule, `parsePodStatusJSON` dependency-free rationale.
  - `docs/architecture/TECHNICAL_ARCHITECTURE.md#3-chartgen-listmap-chartvalues-support` — pre/post state, `helm template` path switch (`--set` vs `-f`), `ErrChartValueConflictingShape`, PR [#361](https://github.com/verity-org/verity/pull/361) image-override precedence preserved + regression-tested.
  - `docs/architecture/TECHNICAL_ARCHITECTURE.md#4-image-entrypoint-conventions-for-imageschartyaml` — when to set/omit `entrypoint:`, the `Entrypoint: null + Cmd: null` contract, argv[0] preservation through both absolute-path and PATH-resolution paths, verification.
  - `ARCHITECTURE.md > Pipeline > Chart-integration smoke tests` — 4-bullet pointer to the technical doc (no duplication).
  - `internal/integer/config/types.go` `PathDef` godoc — documents `hardened-binary` as forwarded-to-apko unvalidated.
- Constraints honored:
  - Conservative diffs — no drive-by re-flow of existing ARCHITECTURE.md sections. New subsection only.
  - No SCR edits.
  - No new top-level docs (TECHNICAL_ARCHITECTURE.md lives under `docs/architecture/` as the SCR AC-9 names explicitly).
  - One commit, message format per spec.
- AC-9 satisfied: **YES** — both `docs/architecture/TECHNICAL_ARCHITECTURE.md` and `ARCHITECTURE.md` cover the `SKIPS.yaml` mechanism AND the retry wrapper (and additionally cover the chartgen list/map extension + image entrypoint convention that emerged during implementation).
- Awaiting subtask 10 (PR + 5-night watch + AC-8 strict-mode flip).
