# Verity — Bundle Copa as an Internal Go Library

Migration plan: replace the `verity-org/copacetic` fork and the shelled-out
`copa` binary with `github.com/project-copacetic/copacetic` imported as a Go
library and exposed via a new `verity patch` subcommand.

---

## 1. Current State

### How verity uses copa today

Verity does **not** import copa as a Go library anywhere. Copa is a separate
binary that verity invokes via shell scripts from GitHub Actions. The complete
touch-surface is small:

| Surface | File(s) | What it does |
|---|---|---|
| CI binary install | `.github/actions/setup-binaries/action.yml` | Clones `verity-org/copacetic` (branches `verity` → `feat/bulk-skip-detection` fallback), runs `go build`, caches the binary |
| CI invocation | `.github/scripts/patch-image.sh` | One `copa patch …` call per image/platform; one OS-only retry on Go-rebuild failure |
| CI workflow | `.github/workflows/patch-image.yaml` | Matrix fan-out (per-image, per-platform) that calls `patch-image.sh` |
| Config parser | `internal/copaconfig.go` | Parses copa's `--output-json`; not called from production code paths (tests only) |
| Config file | `copa-config.yaml` | Lives in verity repo; schema is `copa.sh/v1alpha1` — consumed by **verity** (`internal/discovery/`), not by copa itself |

Verity's own Go module (`github.com/verity-org/verity`) is tiny — `go.mod` is
1.2 KB, depends on only a handful of packages (urfave/cli, yaml.v3). **Adding
copa will materially change the dependency footprint.**

### Fork state (from upstream-side work in prior session)

- `verity-org/copacetic` `verity` branch: 68 commits ahead of
  `project-copacetic/copacetic` `main`, 0 behind. Green build, green tests.
- Three logical buckets on top of upstream:
  1. **Go VCS fallback** — PR #1546 upstream (status: CHANGES_REQUESTED, blocked on #1516).
  2. **Helm chart patching (copa-side)** — PR #1547 upstream (status: CONFLICTING, no reviews in >1 week). **Not needed by verity** (verity does helm in `chart-gen`, not through copa).
  3. **Bulk-engine extras** — `--dry-run`, `--output-json`, `target.registry`, per-arch tag exclusion. No PR. **Not needed by verity** (verity uses GH Actions matrix fan-out, not copa bulk).

### Key enabler: upstream PR #1525 (merged Apr 22)

Upstream migrated copa from `docker/docker/client` → `moby/moby/client`. Net
impact: `go.sum` −369 lines, `go.mod` −111 lines, `docker/docker` now only an
indirect test dep. **Before #1525**, importing copa from another Go module was
a dep-hell non-starter (docker/buildkit/moby version conflicts). **Now it's
practical.** This is the change that unlocks Option B (bundle copa into verity).

---

## 2. Target State

```
┌────────────────────────────────────────────────────────────────────┐
│  verity CLI (single binary)                                        │
│                                                                    │
│  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌─────────────┐    │
│  │ scan      │  │ discover  │  │ chart-gen │  │ patch (NEW) │    │
│  │ (Trivy)   │  │ (config)  │  │ (Helm)    │  │             │    │
│  └───────────┘  └───────────┘  └───────────┘  └──────┬──────┘    │
│                                                       │           │
│                                  imports copa as lib ─┘           │
│                                  github.com/project-copacetic/    │
│                                  copacetic/pkg/patch              │
└────────────────────────────────────────────────────────────────────┘
                    ↓                                    ↓
          BuildKit (via copa)                     GHCR (patched images)
```

- `./verity patch --image X --report Y --tag Z [--go-vcs-url U]` replaces
  the `copa patch` shell call.
- `.github/actions/setup-binaries` drops the Copa build step entirely.
- `.github/scripts/patch-image.sh` calls `./verity patch …` and implements the
  retry logic in Go via the copa library, not via stderr grep.
- `verity-org/copacetic` fork is decommissioned (archived or deleted) once
  in-flight PR #1546 lands upstream and we pin to a clean release.

---

## 3. Scope Decisions (Locked)

| Question | Decision | Rationale |
|---|---|---|
| Separate `verity-copa` Go module vs baked into verity? | **Baked into verity** | User explicitly chose this. No new repo, no new module. |
| Port copa's bulk engine (`PatchFromConfig`) into verity? | **No** | Verity uses GH Actions matrix fan-out for parallelism. Copa's bulk engine would duplicate this. |
| Port copa's helm patching (`PatchChart`) into verity? | **No** | `verity chart-gen` is already pure Go and produces wrapper charts, not in-place patched charts. Different architecture entirely. |
| Keep using `copa-config.yaml` filename/schema? | **Yes** | Already verity's file; schema (`copa.sh/v1alpha1`) is consumed by verity, not copa. Renaming is out of scope. |

---

## 4. Open Decisions

Each decision has a default recommendation. If you accept the defaults, the
plan is executable as-is. Override any you want to change; only **D1 blocks
Phase 1**. D2 and D3 can be answered any time before Phase 5.

| # | Decision | Blocks | Default | Alternatives |
|---|---|---|---|---|
| **D1** | Pin strategy for initial migration | **Phase 1** (need a copa revision to smoke-test) | **Migrate against raw upstream `main` pin, carry a small in-tree patch for Go VCS fallback until PR #1546 merges.** Then re-pin to a tagged release in Phase 5. | (A) pin to an existing upstream commit that happens to include #1546 once it merges — wait-and-see. (B) pin only to tagged releases — forces waiting for both #1546 merge + next tag, likely 2–4 weeks of idle. |
| **D2** | Fork lifecycle after migration | **Phase 5 only** | **Archive `verity-org/copacetic` read-only** — preserves git history, attribution, and any unmerged PR branches for reference. | Delete entirely. |
| **D3** | PR #1546 ownership post-migration | **Phase 5 only** (and only if we want the carry-patch removed) | **Verity team stays owner** — we authored the code, we have the test images, blockers are procedural (waiting on #1516), not technical. | Hand off to a copa maintainer (requires finding one willing; adds coordination cost). |

Phase 1 will proceed under D1's default unless overridden before kickoff.

---

## 5. Architecture

### New package layout

```
cmd/
├── patch.go           NEW  — CLI command: flags, argument parsing, retry flow
└── patch_test.go      EXISTS  — orphan tests already in tree (ready-to-use)

internal/
└── patch/             NEW  — thin orchestrator around copa's public API
    ├── patch.go              — BuildPatchOptions(…), Run(…) → error
    ├── patch_test.go         — unit tests (mock copa)
    └── retry.go              — retry-without-go-vcs-url fallback logic
```

### `verity patch` CLI surface

Mirrors the current shell invocation so CI swap is drop-in:

```
verity patch \
  --image mirror.gcr.io/library/nginx:1.29.3 \
  --tag 1.29.3-linux-amd64-patched \
  --report reports/nginx-1.29.3.json \
  --pkg-types os,library \
  --library-patch-level major \
  --toolchain-patch-level patch \
  --push \
  --buildkit-addr buildx://copa-builder \
  --timeout 30m \
  [--go-vcs-url https://github.com/org/repo@v1.0.0] \
  [--platform linux/amd64]
```

Flag → `copa/pkg/types.Options` field mapping is 1:1 for all ten flags
currently used in `patch-image.sh`. Copa's `types.Options` struct is the
single source of truth — we surface only the fields verity needs.

### Retry logic (in Go, not shell)

Current shell does:
1. Run `copa patch …`
2. If exit != 0 && stderr contains "no package updates found" → treat as success.
3. If exit != 0 && `--go-vcs-url` was provided → retry with `--pkg-types os` and no `--go-vcs-url`.
4. If retry fails the same way → final exit.

In Go via `patch.Patch()`, we inspect the returned `error` type rather than
grepping stderr. Copa's sentinel errors and typed error paths give us a clean
branch in `internal/patch/retry.go`. This is more robust than the shell
grep (which breaks on i18n, log format changes, etc).

### Stream output

`patch.Patch(ctx, opts)` writes diagnostic output through a `*progress.Writer`.
Wire it to `os.Stderr` so GH Actions log output is unchanged. Any structured
result from `patch.Patch` (what was patched, skipped, failed) gets printed as
one summary line for log-grep compatibility.

---

## 6. Migration Phases

### Phase 0 — Prerequisites

Things that must be true before starting Phase 1.

- [x] Upstream PR #1525 (moby/moby/client) merged (Apr 22).
- [x] `verity-org/copacetic` `verity` branch rebased onto upstream/main.
- [ ] **D1 answered** (see §4). D1's default is executable; no action needed unless overriding.
- [ ] Record chosen copa revision (commit hash) in the Phase 1 smoke note (`.sisyphus/notes/copa-import-smoke.md` under the "Pin" section) so the pin is discoverable alongside the other verification artifacts.

D2 and D3 do not block Phase 1. Defer until Phase 5 kickoff.

**QA scenario — verifying Phase 0 is ready to hand off to Phase 1**

- Tool: `bash`
- Steps:
  ```bash
  # Confirm chosen revision exists and is reachable
  git ls-remote https://github.com/project-copacetic/copacetic "$COPA_PIN" | head -1

  # Confirm verity still builds clean before any copa work starts
  go build ./... && go vet ./... && go test ./...
  ```
- Expected: `git ls-remote` prints one line; all three Go commands exit 0.
- On failure: bad pin (retry D1) or pre-existing verity breakage (fix before starting).

### Phase 1 — Import smoke test (½ day)

Goal: prove copa imports cleanly and `patch.Patch()` works end-to-end from a
standalone Go test, before touching any verity code.

Steps:
1. Create a throwaway branch `spike/copa-import`.
2. Add a single Go file `spike/main.go` that imports
   `github.com/project-copacetic/copacetic/pkg/patch` and
   `github.com/project-copacetic/copacetic/pkg/types`.
3. `go mod edit -require=github.com/project-copacetic/copacetic@<chosen-commit>`.
4. Run `go mod tidy`. **Record go.sum line count delta** (expected: +500 to
   +1500 lines; alarm threshold: +3000).
5. Write a minimal `patch.Patch(ctx, &types.Options{…})` call against a tiny
   test image (e.g., `mirror.gcr.io/library/alpine:3.18.0` with a known CVE).
6. Run locally with a BuildKit container; verify a patched image is produced.
7. If the dependency tree looks too large, record the worst offenders and
   decide whether to accept it or file an upstream issue first.

**Deliverable:** a written note in `.sisyphus/notes/copa-import-smoke.md`
recording:
- Exact copa revision tested
- `go.mod` / `go.sum` delta
- Which `types.Options` fields are actually populated by the 10 flags we need
- Any surprises (BuildKit connection setup awkwardness per upstream-side Risk 3)

**QA scenario — Phase 1**

- Tool: `bash` + local Docker registry (`docker-compose.yaml` already wires one on `:5555`) + local BuildKit.
- Pre-step: `make up` to start the local registry and BuildKit container.
- Steps:
  ```bash
  git checkout -b spike/copa-import

  # Snapshot baseline
  wc -l go.sum > /tmp/before.txt

  # Add the import
  go mod edit -require=github.com/project-copacetic/copacetic@$COPA_PIN
  go mod tidy
  wc -l go.sum > /tmp/after.txt

  # Confirm delta budget
  echo "go.sum growth: $(( $(cat /tmp/after.txt | cut -d' ' -f1) - $(cat /tmp/before.txt | cut -d' ' -f1) )) lines"

  # Build + execute the spike
  go build -o spike-patch ./spike
  ./spike-patch  # patches mirror.gcr.io/library/alpine:3.18.0 → localhost:5555/alpine:patched

  # Verify a patched image was produced
  crane digest localhost:5555/alpine:patched
  crane manifest localhost:5555/alpine:patched | jq '.layers | length'
  ```
- Expected:
  - `go.sum` growth between +500 and +1500 lines. Alarm threshold +3000.
  - `spike-patch` exits 0.
  - `crane digest` prints a sha256 digest; `jq` returns ≥ 2 layers.
- On failure:
  - Growth over threshold → inspect with `go mod why -m <module>` on the top 10 biggest additions; decide if acceptable or file upstream issue.
  - Spike binary fails to build → record the exact compile error in the smoke note; if it's a missing exported field, we may need D1 revision bump.
  - Spike runs but no patched image → usually BuildKit addr misconfiguration; confirm `buildx://copa-builder` is reachable via `docker buildx ls`.

**Exit criterion:** QA scenario passes; all three deliverable items written to the smoke note.

### Phase 2 — Implement `verity patch` (1 day)

1. `cmd/patch.go` — CLI command registered in `main.go`. Ten flags matching
   `patch-image.sh`'s current usage. Argument validation (use the existing
   `cmd/patch_test.go` — it already tests image + registry validation).
2. `internal/patch/patch.go` — `BuildOptions(flags) (*types.Options, error)` +
   `Run(ctx, opts) error`. Thin. No business logic beyond calling
   `patch.Patch`.
3. `internal/patch/retry.go` — implements the two-stage retry (with Go VCS →
   without). Uses typed errors from copa, not stderr grep.
4. `cmd/patch_test.go` — expand from current orphan test to exercise the real
   flow with a mock `patch.Patch` (table-driven).

**QA scenario — Phase 2**

- Tool: `bash` + local Docker registry + BuildKit (same setup as Phase 1).
- Steps:
  ```bash
  # Record baseline binary size
  ls -la verity 2>/dev/null && mv verity verity.before 2>/dev/null || true

  # Build with the new patch command
  go build -o verity .
  echo "binary size: $(ls -la verity | awk '{print $5}') bytes"

  # Lint and test
  go vet ./... && go test ./... && golangci-lint run

  # Verify CLI surface
  ./verity patch --help | tee /tmp/patch-help.txt
  for flag in image tag report pkg-types library-patch-level toolchain-patch-level push buildkit-addr timeout go-vcs-url; do
    grep -q "\-\-$flag" /tmp/patch-help.txt || { echo "MISSING FLAG: --$flag"; exit 1; }
  done

  # End-to-end against a real (tiny) image
  ./verity patch \
    --image mirror.gcr.io/library/alpine:3.18.0 \
    --tag alpine-smoke-patched \
    --report /tmp/alpine-trivy.json \
    --pkg-types os \
    --push \
    --buildkit-addr buildx://copa-builder \
    --timeout 10m

  crane digest localhost:5555/alpine:alpine-smoke-patched
  ```
- Expected:
  - `go build` / `go vet` / `go test` / `golangci-lint` all exit 0.
  - `--help` output contains all 10 required flags (loop above exits 0).
  - Final `crane digest` prints a sha256.
  - Binary size growth ≤ 2× (recorded in PR description).
- On failure:
  - Missing flag → fix `cmd/patch.go` flag definitions.
  - Test failures → expected `patch.Patch` mock shape mismatch; align with actual copa signature.
  - Real patch fails → check `/tmp/alpine-trivy.json` exists and is valid Trivy output.

**Exit criterion:** QA scenario passes; PR description records binary size delta.

### Phase 3 — Switch CI to `verity patch` (½ day)

1. Edit `.github/scripts/patch-image.sh`: replace `copa patch "${COPA_ARGS[@]}"`
   with `./verity patch …`. Keep the retry wrapper for now (belt + braces).
2. Edit `.github/actions/setup-binaries/action.yml`: **remove the entire
   Copa Setup section** (lines ~28–72). Verity setup stays.
3. Edit `.github/workflows/patch-image.yaml`: ensure verity binary is built
   before `patch-image.sh` runs (already the case — job just needs the order
   verified).
4. Smoke-test on a PR against a single tiny image.

**QA scenario — Phase 3**

- Tool: `gh` CLI + GitHub Actions UI.
- Steps:
  ```bash
  # 1. Confirm CI action no longer references verity-org/copacetic
  grep -rn 'verity-org/copacetic\|/copa ' .github/ || echo "CLEAN"

  # 2. Dispatch patch-image workflow against a small test image
  gh workflow run patch-image.yaml \
    -f image-name=alpine \
    -f source-ref=mirror.gcr.io/library/alpine:3.18.0 \
    -f target-registry=ghcr.io/verity-org \
    -f platforms=linux/amd64

  # 3. Capture the run ID and wait
  RUN_ID=$(gh run list --workflow=patch-image.yaml --limit=1 --json databaseId -q '.[0].databaseId')
  gh run watch "$RUN_ID"

  # 4. Inspect the patch step logs for the new binary in use
  gh run view "$RUN_ID" --log | grep -E 'verity patch|copa patch' | head -5

  # 5. Verify the patched image exists in GHCR
  crane digest ghcr.io/verity-org/alpine:3.18.0-linux-amd64-patched
  ```
- Expected:
  - Step 1: `grep` prints `CLEAN` (or only historical doc mentions).
  - Step 2: workflow dispatch accepted (exit 0).
  - Step 3: workflow run completes with status `success`.
  - Step 4: log lines show `verity patch …` invocations, zero `copa patch …` invocations.
  - Step 5: `crane digest` returns a sha256.
- On failure:
  - Step 1 finds live references → grep output shows files that still need editing.
  - Step 3 fails in `scan` job → pre-existing issue, not migration-caused; investigate separately.
  - Step 3 fails in `patch` job → inspect `gh run view --log` for the verity patch stderr; common cause is BuildKit addr mismatch or missing required flag mapping.
  - Step 5: digest missing → patched image didn't push; check `--push` flag was set.

**Exit criterion:** all 5 QA steps green; no `copa` binary installed on the runner (`which copa` returns non-zero inside the workflow).

### Phase 4 — Remove shell retry wrapper (optional, ½ day)

Once Phase 3 is green on 3+ images across both amd64 and arm64, move the
retry logic from shell to Go (per §5 Retry logic). Simpler error paths,
easier to test, no regex-on-stderr fragility.

1. Delete the retry shell block from `patch-image.sh`.
2. Move fallback logic to `internal/patch/retry.go`.
3. Add unit test for the retry branch.
4. Smoke-test again on the orchestrator.

**QA scenario — Phase 4**

- Tool: `bash` (unit test) + `gh` (integration).
- Steps:
  ```bash
  # 1. Unit test: retry path triggers correctly
  go test ./internal/patch/... -run TestRetry -v

  # 2. Shell script shrink check
  wc -l .github/scripts/patch-image.sh

  # 3. Integration: dispatch against a Go-binary image known to need VCS fallback
  gh workflow run patch-image.yaml \
    -f image-name=prometheus \
    -f source-ref=quay.io/prometheus/prometheus:v3.9.1 \
    -f target-registry=ghcr.io/verity-org \
    -f go-vcs-url=https://github.com/prometheus/prometheus@v3.9.1 \
    -f platforms=linux/amd64
  RUN_ID=$(gh run list --workflow=patch-image.yaml --limit=1 --json databaseId -q '.[0].databaseId')
  gh run watch "$RUN_ID"

  # 4. Inspect logs for the retry-without-vcs branch
  gh run view "$RUN_ID" --log | grep -E 'retry without go-vcs|fallback to os-only'
  ```
- Expected:
  - Step 1: unit tests exit 0 with ≥ 2 retry scenarios covered (primary succeeds / retry succeeds / retry fails).
  - Step 2: line count ≤ 25 (down from ~75).
  - Step 3: workflow succeeds.
  - Step 4: if the Go rebuild had any issues the log shows the retry path; otherwise acceptable to show the primary path succeeded.
- On failure:
  - Step 1 fails → retry logic has a branch bug; do NOT proceed to integration.
  - Step 3 fails on primary path → regression in `verity patch`, not retry-specific; bisect against Phase 3 main.
  - Step 3 fails on retry path → the typed-error mapping from copa doesn't match what we assumed; inspect actual error with a debug log and refine the branch condition.

**Exit criterion:** `patch-image.sh` is ~20 lines (just env var wiring + one
`./verity patch` invocation), unit tests green, one orchestrator run showing
either primary or retry success.

### Phase 5 — Decommission fork (½ day, after PR #1546 merges upstream)

This can happen days or weeks later, depending on upstream merge velocity.

1. Bump copa import pin from chosen-commit to a released tag that includes
   #1546.
2. Drop any in-tree carry-patch for the Go VCS fallback (per D1).
3. Update `README.md` / `ARCHITECTURE.md` acknowledgments: "Verity maintained
   a fork from YYYY to YYYY-MM; now consumes upstream Copa directly."
4. Archive `verity-org/copacetic` (or delete per D2).
5. Delete stale verity branches (`feat/go-patching`, `feat/go-patching-upstream`,
   `fix/distroless-go-vcs-fallback`, `fix/go-rebuild-placeholder`) — they're
   superseded by upstream PR #1388.

**QA scenario — Phase 5**

- Tool: `bash` + `gh api`.
- Steps:
  ```bash
  # 1. Zero active references to the fork in source/config/docs (excluding the plan itself)
  MATCHES=$(grep -rn 'verity-org/copacetic' \
    --include='*.yaml' --include='*.yml' \
    --include='*.go' --include='*.sh' \
    --include='*.md' --include='Makefile' \
    --include='Dockerfile*' \
    . | grep -v '^\./\.sisyphus/' | wc -l)
  echo "live references: $MATCHES"
  [ "$MATCHES" -eq 0 ] || echo "FAIL: still referenced above"

  # 2. go.mod is pinned to a tagged release (not a raw commit)
  grep 'project-copacetic/copacetic' go.mod
  # Expected format: github.com/project-copacetic/copacetic vX.Y.Z

  # 3. Carry-patch removed (if any was added in Phase 1)
  ls replace/ 2>/dev/null && echo "UNEXPECTED: carry-patch directory still exists"
  grep -E '^replace.*copacetic' go.mod && echo "FAIL: replace directive still in go.mod"

  # 4. Fork repo is archived on GitHub
  gh api repos/verity-org/copacetic --jq '.archived'  # Expected: true

  # 5. Stale branches deleted
  for branch in feat/go-patching feat/go-patching-upstream fix/distroless-go-vcs-fallback fix/go-rebuild-placeholder; do
    gh api "repos/verity-org/copacetic/branches/$branch" 2>/dev/null && echo "FAIL: $branch still exists" || echo "ok: $branch deleted"
  done
  ```
- Expected:
  - Step 1: `live references: 0`.
  - Step 2: `go.mod` line matches `vX.Y.Z` (semver tag), not a commit hash.
  - Step 3: no `replace/` directory; no `replace` directive for copa in `go.mod`.
  - Step 4: `gh api` returns `true`.
  - Step 5: all four branches return `ok: … deleted`.
- On failure:
  - Step 1 > 0 → the grep output shows exactly what to edit. Re-run after each fix.
  - Step 2 not a tag → the chosen copa release didn't ship yet, or D1 override means we're staying on commit-pin; document why.
  - Step 3 residue → Phase 5 missed the removal; check git log for when carry-patch was added.
  - Step 4 false → user hasn't run the archive action yet; not a code issue.

**Exit criterion:** all 5 QA steps green.

---

## 7. Risks & Mitigations

| # | Risk | Likelihood | Severity | Mitigation |
|---|---|---|---|---|
| R1 | **Dep tree bloat** — copa brings in buildkit, containerd, moby, syft, trivy-db libs. Binary may double in size. | High | Medium | Measure in Phase 1 smoke test. If binary > 60 MB, investigate `-ldflags='-s -w'` and build-tag-gated feature exclusion. |
| R2 | **Copa's `types.Options` API is unstable** — fields may be renamed post-#1525. | Medium | Medium | Pin to an exact commit. Update intentionally with a dedicated PR. Any rename is a compile error, not a runtime surprise. |
| R3 | **BuildKit connection setup awkwardness** — copa's library API may require verity to manage BuildKit's `client.Client` explicitly. | Medium | Low | Confirm in Phase 1 smoke test. If awkward, file an upstream issue before Phase 2. |
| R4 | **Retry fallback less reliable in Go than shell** — if we move it per Phase 4, wrong error type branching could skip a legitimate retry. | Low | Medium | Keep the shell retry in place through Phase 3. Only move to Go once we have 2+ weeks of production runs confirming the error taxonomy. |
| R5 | **Loss of `copa` CLI as independent debug tool** — developers used to run `copa patch` locally. | Low | Low | `./verity patch` is drop-in equivalent. Document in `CONTRIBUTING.md`. If absolutely needed, `go run github.com/project-copacetic/copacetic@<pin>` still works for ad-hoc use. |
| R6 | **PR #1546 never lands upstream** — blocks D1 option B indefinitely. | Medium | Low | Phase 0 picks option C (migrate now, carry small diff). Migration proceeds regardless of upstream timeline. |
| R7 | **Two BuildKit connection paths in CI** — the workflow currently sets up `copa-builder` via `docker buildx create`. Must keep that, just pointed at by `./verity patch` instead. | Low | Low | Keep `--buildkit-addr buildx://copa-builder` flag; no workflow changes required. |
| R8 | **Binary rebuilds vs cache** — CI currently caches the prebuilt `copa` binary keyed on commit. After migration, the cache becomes `go/pkg/mod` for the copa lib. Cache structure changes. | High | Low | Expected. Drop the Copa cache key entirely. Verity build uses existing Go module cache. |

---

## 8. Time Estimate

| Phase | Effort | Blocker? |
|---|---|---|
| 0 — Prerequisites | 0 (done + decisions) | D1/D2/D3 answers |
| 1 — Smoke test | 0.5 day | — |
| 2 — `verity patch` impl | 1 day | Phase 1 pass |
| 3 — CI switch | 0.5 day | Phase 2 merged |
| 4 — Shell → Go retry | 0.5 day | optional |
| 5 — Decommission fork | 0.5 day | PR #1546 upstream merge |
| **Total (Phases 1-3)** | **~2 days** | — |
| **Total (Phases 1-5)** | **~3 days + wall-clock wait for #1546** | — |

---

## 9. Success Metrics

1. `/copa` and `verity-org/copacetic` strings return zero hits in
   `grep -r --include='*.yaml' --include='*.sh' --include='*.go' --include='*.yml' .`
   (excluding historical docs mentions).
2. `./verity patch --help` works; flag signature matches Section 5.
3. One full `orchestrator.yaml` run patches ≥ 1 image end-to-end using
   `verity patch` with no `copa` binary present.
4. `ARCHITECTURE.md` "Components" table lists Copa as "library dependency"
   not "external binary".
5. `go.mod` references `github.com/project-copacetic/copacetic` at a pinned
   version; `go.sum` is in the repo.

---

## 10. Out of Scope

Explicitly **not** part of this migration:

- Porting any code from the verity fork's bulk-engine modifications. Verity
  doesn't use copa bulk.
- Porting any code from the verity fork's helm-chart-patching. Verity's
  `chart-gen` is a different architecture that works.
- Rewriting `copa-config.yaml` schema. Config stays where and how it is.
- Replacing Trivy. Out of scope; different PR entirely.
- Replacing BuildKit setup in CI. Current docker-buildx wiring stays; only
  the binary that connects to it changes.
- Any upstream copa PR work (PR #1546 finish, PR #1547 outcome). Those are
  separate work items owned by the copa-side session.

---

## 11. Links & References

- Upstream: https://github.com/project-copacetic/copacetic
- Fork (to be decommissioned): https://github.com/verity-org/copacetic
- Blocking PR (Go VCS fallback): https://github.com/project-copacetic/copacetic/pull/1546
- Prerequisite PR (moby migration, merged): https://github.com/project-copacetic/copacetic/pull/1525
- Current `verity patch` orphan tests: `cmd/patch_test.go`
- Current copa invocation: `.github/scripts/patch-image.sh` lines 33–75
- Current copa binary install: `.github/actions/setup-binaries/action.yml` lines 28–72
