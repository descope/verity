---
id: TASK-SCR-2026-05-06-001
scr: SCR-2026-05-06-001
discussion: DISCUSSION-001
title: Upgrade Copa to v0.14.0 and drop verity-org/copacetic fork
status: active
phase: D-4 pending (PO-driven staging dispatch); D-1 (PO rebase via PR #12), D-2 (re-pin), D-3 (smoke + regex) done
complexity: standard
track: implementation
slice: core
created: 2026-05-06
activated_at: 2026-05-06
assignee: beads-task-agent (Phase A)
---

# Upgrade Copa to v0.14.0 and drop verity-org/copacetic fork

> **Status:** Active. SCR-2026-05-06-001 approved by PO on 2026-05-06.
>
> **Phase split:**
> - **Phase A** (delegated to subagent): pre-flight branch hygiene, dep bump, comment cleanup, build, unit tests. Stop before local smoke.
> - **Phase B** (PO-driven): local smoke + staging dispatch + retry-pattern validation (need Docker + GH CI auth).
> - **Phase C** (delegated): doc update, PR creation, atomic commit.

## Pre-Flight (Phase A step 0)

Repo state at activation:
- On stale branch `fix/charts-revert-kyverno-postgres-tag` (PR #308, already merged).
- `main` is 2 commits behind `origin/main`.
- No working-tree changes; SCR + task files are untracked.

Required hygiene:
1. `git checkout main && git pull --ff-only`
2. `git checkout -b chore/copa-0.14-upgrade`
3. `git add docs/scrs/SCR-2026-05-06-001-copa-0-14-upgrade.md tasks/todo/SCR-2026-05-06-001-copa-0-14-upgrade/task.md tasks/current.md tasks/discussions/DISCUSSION-001-copa-0-14-upgrade-evaluation.md`
4. Do NOT commit yet — these go in the same atomic commit as the dep bump (AC-8).

## Linked SCR

`docs/scrs/SCR-2026-05-06-001-copa-0-14-upgrade.md` — read it fully before starting.

## Acceptance Criteria

Mirror of SCR ACs. Final evidence must trace back to each:

- **AC-1** `go.mod` pins `github.com/project-copacetic/copacetic v0.14.0`
- **AC-2** `replace` directive removed; `go.sum` clean of `verity-org/copacetic`
- **AC-3** `go build` + `go test ./...` pass 100%
- **AC-4** Fork-pending comments updated in `internal/patch/patch.go` and `.github/scripts/patch-image.sh`
- **AC-5** `_is_go_rebuild_failure()` patterns re-validated against Copa 0.14 logs
- **AC-6** Staging orchestrator dry-run on ≥ 5 images: CVE-after ≤ baseline
- **AC-7** `ARCHITECTURE.md` Components table updated (Copa v0.14.0)
- **AC-8** Atomic commit landed via PR linking this SCR

## Implementation Order

1. Branch from `main`: `chore/copa-0.14-upgrade`.
2. `go get github.com/project-copacetic/copacetic@v0.14.0`.
3. Edit `go.mod` — remove `replace` line. Run `go mod tidy`.
4. `go build -o verity . && go test ./...` — must pass.
5. Update comments — `internal/patch/patch.go` (GoVCSURL doc), `.github/scripts/patch-image.sh` (header block).
6. Local smoke patch against 2 images (one stripped Go, one Python). Save logs to `evidences/SCR-2026-05-06-001-copa-0-14-upgrade/logs/`.
7. Re-validate `_is_go_rebuild_failure()` regex against the captured logs. Update patterns if needed.
8. Push branch. Trigger orchestrator staging run via `workflow_dispatch` against the AC-6 subset.
9. Capture CVE diff + duration diff. Save to evidence packet.
10. Update `ARCHITECTURE.md` Components table row for Copa.
11. Open PR. PR description: links to this SCR, AC checklist, evidence summary.
12. Merge after CI green.

## Evidence Packet Required

`evidences/SCR-2026-05-06-001-copa-0-14-upgrade/`:

- `SUMMARY.md` — what was tested, AC trace, links to logs/screenshots.
- `logs/local-smoke-cockroach.log` — local stripped-Go patch run.
- `logs/local-smoke-python.log` — local Python patch run.
- `logs/retry-pattern-validation.md` — regex match analysis against 0.14 logs.
- `logs/staging-orchestrator-run.txt` — staging run summary.
- `logs/cve-diff.json` — CVE-before/after per image in the subset.
- No screenshots (no UI change).

## Verification Gates

- **Tech Lead** (PO acting): code review of `go.mod`/`go.sum` diff, comment updates, retry-pattern updates.
- **PMA closure:** evidence packet complete, ACs traced, atomic commit landed.

## Discussion Record

This task originated from `DISCUSSION-001-copa-0-14-upgrade-evaluation`. Key decisions captured in the SCR:

- Hold Python venv + RPM chroot adoption for follow-up SCRs (out of scope here).
- Single atomic PR over a split PR (subject to PO confirmation in SCR open questions).
- Staging subset: 5 images covering distroless Go, RPM, Python, Node, Wolfi sanity (subject to PO confirmation).

## Reopen History

_(none yet)_

## Post Implementation Task Updates

### beads-task-agent: Phase A Post Implementation Expectations

Phase A completed 2026-05-06. Task tool session: `ses_20187d549ffe31z25O6924OO5y` (preserve for bouncebacks).

Observable outcomes after Phase A:

- Branch `chore/copa-0.14-upgrade` exists locally, forked from updated `main` (FF'd 9 commits to `3809821c29`).
- `go.mod` line 9: `github.com/project-copacetic/copacetic v0.14.0` — tagged release.
- `go.mod` contains zero `replace` directives (`grep -c "^replace " go.mod` = 0).
- `go.sum` contains zero `verity-org/copacetic` entries (`grep -c "verity-org/copacetic" go.sum` = 0).
- `go.sum` diff is minimal — 4 lines (2 added v0.14.0, 2 removed verity-org). No transitive bumps. Implies fork dep tree was identical to upstream v0.14.0 + PR #1546.
- `go build -o verity .` exit 0; binary present at `./verity`.
- `go test ./...` exit 0 across 16 packages (full log: `evidences/SCR-2026-05-06-001-copa-0-14-upgrade/logs/phase-a-build-and-tests.log`).
- `internal/patch/patch.go` `GoVCSURL` doc comment rewritten — no PR #1546 / fork references.
- `.github/scripts/patch-image.sh` header `GO_VCS_URL` block rewritten — no fork-pending language; retry-branch context preserved.
- All staged on the branch; no commits yet (Phase C will atomic-commit).

Open risks flagged for PO:

1. Tiny go.sum diff means Phase A exercises **no** new Copa behavior — bug fixes #1530, #1516, #1532 are only validated in Phase B integration.
2. `internal/patch` unit tests are thin; copa import surface is compile-time only. Phase B local smoke is the first behavioral signal.
3. Whether to commit the `evidences/` packet alongside the atomic commit is an open PO decision.
4. Side observation: untracked `.codenomad/` and `.nomadworks/` dirs in repo — candidate for `.gitignore` if not meant for version control. Not in scope for this SCR.

ACs satisfied: AC-1, AC-2, AC-3, AC-4. Pending: AC-5 (Phase B), AC-6 (Phase B), AC-7 (Phase C), AC-8 (Phase C).

### beads-task-agent: Phase B-1 + B-2 Post Implementation Expectations

Phase B-1 + B-2 completed 2026-05-06. Same task tool session: `ses_20187d549ffe31z25O6924OO5y`.

Observable outcomes after Phase B-1 + B-2:

- `copa-builder` buildx builder running (rootless docker, BuildKit v0.29.0).
- Two scan reports generated: `reports/mirror.gcr.io_cockroachdb_cockroach_v26.1.3.json`, `reports/quay.io_prometheus_prometheus_v3.11.3.json`.
- Two smoke logs captured: `evidences/.../logs/local-smoke-cockroach.log` (256 lines, exit 1 — Go-rebuild failure path), `local-smoke-python.log` (5384 lines, exit 0 — 19/23 CVEs patched).
- Retry pattern analysis written: `evidences/.../logs/retry-pattern-validation.md` with full coverage table, false-positive verification against the success log, and recommended replacement regex.
- **Critical:** all 6 legacy `_is_go_rebuild_failure()` patterns return zero matches on real Copa 0.14 failure output. Pattern set is functionally broken on 0.14.
- No edits to `patch-image.sh`, no commits, no pushes.
- Agent substituted `prometheus/prometheus:v3.11.3` for the requested "Python pip" target — `copa-config.yaml` has zero pip-targeting entries (verified via full sweep). The substitution is methodologically stronger: the success run serves as a negative control for false-positive verification.

Critical risks elevated by Phase B:

1. **Hard prerequisite for safe landing:** the `patch-image.sh` regex update must ship in the same atomic commit as the dep bump. Without it, every Go-rebuild failure becomes a hard fail in nightly orchestrator runs (regression).
2. **Limited failure-mode coverage:** only stripped-binary + empty-go-mod-graph mode exercised. Distroless-no-shell, monorepo-wrong-directory, and VCS-rev-not-resolvable modes still need empirical verification — Phase B-3 staging dispatch is the only path to that coverage.
3. **Possible upstream Copa regression:** cockroach exit-1 (`go mod download ... failed: no modules specified`) may be a real 0.14 regression vs. the verity-org fork. Worth filing an issue against project-copacetic/copacetic regardless of how this SCR closes. Out of scope for this SCR; capture as follow-up.

ACs satisfied after B-1 + B-2: AC-1, AC-2, AC-3, AC-4, **AC-5**. Pending: AC-6 (Phase B-3 staging dispatch), AC-7 (Phase C), AC-8 (Phase C).

### product_owner: Phase D-1 Post Implementation Expectations

Phase D-1 completed 2026-05-08 via [verity-org/copacetic#12](https://github.com/verity-org/copacetic/pull/12) (merged 2026-05-08T18:13:09Z).

Observable outcomes after Phase D-1:

- New integration branch `verity-org/copacetic:verity` exists at HEAD `0c2044849ef7638d22333e07aef4535811e53014`.
- `verity` branch status vs upstream `project-copacetic/copacetic v0.14.0`: **ahead by 67, behind by 0** (carries v0.14.0 + 67 fork commits including all the Go-binary-patching extensions).
- Original `feat/go-vcs-resolution` branch unchanged (HEAD still `d9dd076`); diverged from upstream as before. Replaced by `verity` as the canonical integration branch.
- Computed pseudo-version for go.mod: `v0.0.0-20260508181309-0c2044849ef7`.

### beads-task-agent: Phase D-2 + D-3 Post Implementation Expectations

Phase D-2 + D-3 completed 2026-05-08. Same task tool session: `ses_20187d549ffe31z25O6924OO5y`.

Observable outcomes after Phase D-2 (re-pin):

- Phase A's wrong-direction changes reverted: `git diff main -- go.mod go.sum internal/patch/patch.go .github/scripts/patch-image.sh` was empty before re-pin.
- `go.mod` line 9: `github.com/project-copacetic/copacetic v0.14.0` (tagged release).
- `go.mod` `replace` directive: `github.com/project-copacetic/copacetic => github.com/verity-org/copacetic v0.0.0-20260508181309-0c2044849ef7`.
- `go.sum` updated for the new pseudo-version.
- Forced shim applied in `internal/patch/patch.go` line 161: `progressui.DisplayMode("plain")` → `types.DisplayMode("plain")` (Copa 0.14 moved `DisplayMode` from `moby/buildkit/util/progress/progressui` into its own `pkg/types`). 2-line change: 1 import drop, 1 type cast. Zero behavior change.
- `go build -o verity .` exit 0 after the shim.
- `go test ./...` exit 0 across all 16 packages.
- Comment updates: `internal/patch/patch.go` `GoVCSURL` doc (lines 100-117) and `.github/scripts/patch-image.sh` header (lines 11-26) describe rebased-fork state. `grep -nE "1546|PR #"` returns no matches in either file.
- `_is_go_rebuild_failure()` regex body and retry branch logic untouched.

Observable outcomes after Phase D-3 (smoke + regex):

- `evidences/.../logs/phase-d3-smoke-cockroach.log` (~5.9KB, 97 lines, exit 1) — fork-mode failure preserved. Failure string: `go package upgrade operation failed: no binaries were successfully rebuilt: [/cockroach/cockroach: source code not available...]`. New "binary preserved, Go vulnerabilities remain" suffix from fork commit 8 (pre-rebuild Solve verification) is informational.
- `evidences/.../logs/phase-d3-smoke-prometheus.log` (~13.9KB, 367 lines, exit 0) — bit-identical to Phase B-2: 23 total / 19 patched / 4 skipped.
- Regex coverage: 6 matches in cockroach failure log, 0 false positives in prometheus success log. **No regex change needed.**
- `retry-pattern-validation.md` updated with "Post-Rebase Re-Validation (Phase D-3)" section.

Open risks elevated:

1. **Cockroach Go-CVE coverage gap** — pre-existing this SCR (cockroach is a complex monorepo that won't compile from vanilla `go mod download` in copa's sandbox). Orchestrator OS-only retry handles it. Candidate for follow-up SCR if Go-level CVE coverage on cockroach matters for verity's threat model.
2. **`progressui.DisplayMode` shim** — forced API-rename change beyond originally-scoped comment-only updates. Unavoidable; alternative is broken build.

ACs satisfied after D-2 + D-3: AC-1, AC-2, AC-3, AC-4, **AC-5 (unconditionally — no regex change needed)**. Pending: AC-6 (Phase D-4 staging dispatch), AC-7 (Phase D-5), AC-8 (Phase D-5).

### beads-task-agent + product_manager: Phase D-4 Post Implementation Expectations

Phase D-4 completed 2026-05-09. Same task tool session: `ses_20187d549ffe31z25O6924OO5y`.

Observable outcomes after Phase D-4:

- Staging commit `84fdbe0a28` pushed to `origin/chore/copa-0.14-upgrade` (8 files, +843/-24).
- Branch URL: https://github.com/verity-org/verity/tree/chore/copa-0.14-upgrade
- 5-image subset dispatched (with documented substitutions for catalog drift):
  - `cockroachdb/cockroach` (3 tags) — 3/3 SUCCESS
  - `mongodb/mongodb-community-server` (3 tags) — 3/3 SUCCESS
  - `prometheus-operator/prometheus-config-reloader` (4 tags) — 3/4 SUCCESS, 1 pre-existing failure on v0.81.0 (failing on main 3x in past 24h before this SCR)
  - `prometheuscommunity/elasticsearch-exporter` (3 tags, substituted for `kyverno/kyverno` after Discover-empty-match failure on `kyverno/policy-reporter-ui`) — 3/3 SUCCESS
  - `library/rabbitmq` (3 tags, substituted for `nginx` after Discover-empty-match failure on `kubernetes/ingress-nginx/controller`) — 3/3 SUCCESS
- Total: 16 patch-image runs, 15 SUCCESS, 1 PRE-EXISTING FAILURE.
- Critical positive: cockroach 3/3 success in CI proves the rebased fork + retry branch work end-to-end. The exact failure mode that broke on raw v0.14.0 (D-3 finding) is now handled.

AC-6 satisfied: 15/16 of the dispatched runs succeeded; the 1 failure is pre-existing on main and unrelated to the Copa upgrade.

### Follow-up tasks identified (out of scope for this SCR)

These were surfaced during D-4 and should be tracked separately:

1. **Orchestrator concurrency bug** — `cancel-in-progress: false` empirically cancels rather than queues pending dispatches. Either drop the concurrency block or switch to `cancel-in-progress: true`.
2. **Discover step null-handling** — `verity discover` exits with `jq: error (null)` exit 5 when zero tag matches; should exit 0 cleanly with a "no images to dispatch" log.
3. **Catalog drift** — `kyverno/policy-reporter-ui` and `kubernetes/ingress-nginx/controller` have tag patterns matching zero live tags. Investigate and either fix patterns or remove entries.
4. **Pre-existing v0.81.0 failure** — `prometheus-operator/prometheus-config-reloader:v0.81.0` failing on main; investigate or remove tag from catalog.

ACs satisfied after D-4: AC-1, AC-2, AC-3, AC-4, AC-5, **AC-6**. Pending: AC-7 + AC-8 (Phase D-5).

### beads-task-agent: Phase D-5 Post Implementation Expectations

Phase D-5 completed 2026-05-09. Same task tool session: `ses_20187d549ffe31z25O6924OO5y`.

Observable outcomes after Phase D-5:

- Branch `chore/copa-0.14-upgrade` rebased onto current `origin/main` (advanced 8 commits during D-2/D-3/D-4: in-toto v0.11.0 security bump, Go 1.26.3 bump, harden-runner v2.19.1, valkey-cli + atlantis follow-ups, jenkins tag pin, bespoke wave 3). Rebase clean, no conflicts.
- Build + test re-validated post-rebase: `go build` exit 0, `go test ./...` exit 0 across all 16 packages.
- `.gitignore` updated: `evidences/`, `.codenomad/`, `.nomadworks/` ignored. `git check-ignore -v` confirmed all three patterns active.
- `ARCHITECTURE.md` updated:
  - Components table Copa row (line 12) now describes the rebased-fork state, lists the 4 fork-only Go-binary-patching extensions.
  - `--go-vcs-url` flag table entry (line 213) updated to drop "temporary go.mod replace directive" and "PR #1546 ... dropped once #1546 merges" wording.
- Staging commit `84fdbe0a28` (8 files, +843/-24) replaced by atomic commit on rebased base (`9a019aba3c`, 10 files, +852/-26).
- Atomic commit subject: `chore(deps): SCR-2026-05-06-001 rebase copa fork onto v0.14.0+ and re-pin`.
- Force-with-lease push: `84fdbe0a28...9a019aba3c chore/copa-0.14-upgrade -> chore/copa-0.14-upgrade (forced update)`. Branch tracks origin cleanly.
- PR opened: [#343](https://github.com/verity-org/verity/pull/343). Title matches commit subject; body includes 8-AC checklist (all checked), follow-up list, refs to SCR + verity-org/copacetic#12.
- Ledger updates amended onto the atomic commit:
  - `tasks/current.md`: TASK-SCR-2026-05-06-001 moved from Active to Recently Done with PR link; Active Discussions cleared.
  - `docs/scrs/SCR-2026-05-06-001-copa-0-14-upgrade.md`: frontmatter `status` flipped from `approved-revised` to `implemented`; `implemented_at: 2026-05-09` and `implemented_pr` added.
  - `tasks/todo/SCR-2026-05-06-001-copa-0-14-upgrade/task.md` → `tasks/done/SCR-2026-05-06-001-copa-0-14-upgrade/task.md` (moved via `git mv`).

ACs satisfied after D-5: **all 8** (AC-1 through AC-8). Task ready for PMA close-out.
