# Copa Library Import Smoke Test

Date: 2026-04-23
Branch: `feat/verity-patch-bundle-copa`

## Pin

`github.com/project-copacetic/copacetic v0.13.1-0.20260422213235-21047038c9fe`

This is Go's pseudo-version for upstream main `HEAD=21047038c9fe`, timestamped
2026-04-22 21:32:35 UTC. Chosen per plan D1 default: upstream main HEAD
includes PR #1525 (moby/moby/client migration, merged 2026-04-22) which
removes the docker/docker → moby/moby/client type conflict that previously
blocked copa-as-a-library. v0.13.0 was released 2026-01-09 and is 64 commits
behind this pin; it predates PR #1525 and will not build.

### `replace` Directive for Go VCS Fallback (temporary)

Upstream copa HEAD does not yet have `types.Options.GoVCSURL` — that field
arrives with PR #1546 (still open upstream, blocked on rebase). Because
verity actively uses `goVcsUrl` for 16+ images in `copa-config.yaml`
(cert-manager, loki, consul, prometheus, victorialogs, rabbitmq operators,
grafana-operator, gatekeeper, cloud-on-k8s, postgres-operator, ollama, …),
shipping the migration without that field would silently lose Go CVE
patching for those images.

To preserve parity, `go.mod` carries a temporary replace directive:

```
replace github.com/project-copacetic/copacetic => github.com/verity-org/copacetic v0.0.0-20260424111537-fd4ff4a74837
```

`fd4ff4a7` is the HEAD of `verity-org/copacetic feat/go-vcs-resolution`,
which is 10 commits ahead / 0 commits behind upstream main (i.e., upstream
main + exactly the PR #1546 commits). The branch was authored by the
verity team and is what was filed upstream as PR #1546.

**Exit path:** once PR #1546 merges upstream and we re-pin to a tagged
copa release, delete the replace directive in one line. No other code
changes needed — `Options.GoVCSURL` becomes available directly from the
upstream module.

## go.mod / go.sum Delta

| Metric | Before | After | Delta |
|---|---|---|---|
| go.sum line count | 73 | 790 | **+717** |
| Direct requires | 5 | 6 | +1 (copa) |
| Binary size | ~15 MB | ~39 MB | +24 MB (~2.6x) |

Delta is below the plan's +3000 alarm threshold and the expected +1500 upper
bound. Binary growth is the main cost: 15 MB → 39 MB, largely from buildkit,
containerd, moby client, trivy-db, and opentelemetry packages that copa pulls
in transitively.

## Options Fields Actually Used

From the 21 exported fields on `types.Options`, verity populates:

| Field | Config source | Default |
|---|---|---|
| `Image` | `--image` | required |
| `PatchedTag` | `--tag` | required |
| `Report` | `--report` | required |
| `Scanner` | hardcoded `"trivy"` | required (copa's library regex-validates non-empty; CLI-layer default is not applied by library) |
| `PkgTypes` | `--pkg-types` | `"os,library"` |
| `LibraryPatchLevel` | `--library-patch-level` | `"patch"` |
| `ToolchainPatchLevel` | `--toolchain-patch-level` | `"patch"` |
| `Push` | `--push` | `false` |
| `BkAddr` | `--buildkit-addr` | empty (copa auto-detects) |
| `Timeout` | `--timeout` | `0` (copa default) |
| `Platforms` | `[--platform]` (slice of one) | empty (copa default) |
| `Progress` | hardcoded `progressui.DisplayMode("plain")` | for CI log compatibility |

Fields NOT wired (left at copa's zero value): Suffix, ConfigFile,
WorkingFolder, IgnoreError, Format, Output, BkCACertPath, BkCertPath,
BkKeyPath, Loader, OCIDir, OutputContext, EOLAPIBaseURL, ExitOnEOL.

## Surprises

1. **`Scanner` is required at library level, not defaulted like the CLI does.** 
   Empty `Scanner` yields `"invalid scanner name ...: must match ^[a-zA-Z0-9]..."`.
   Fix: hardcode `"trivy"` in `toOptions()` via `defaultScanner` const.

2. **`Timeout == 0` fails immediately.** Copa calls
   `context.WithTimeout(ctx, opts.Timeout)` unconditionally. A zero duration
   produces an already-expired context and copa returns
   `"patch exceeded timeout 0s"` before any work starts. Fix: the CLI flag
   `--timeout` now has `Value: defaultPatchTimeout` (5m, matching legacy
   `copa patch`'s Cobra default). `TestPatchCommandDefaults` locks this in.
   CI always passes `--timeout 30m`, so the regression was invisible there.

3. **`GoVCSURL` field on `types.Options` requires a `replace` directive.**
   Upstream copa HEAD lacks this field; PR #1546 (authored by the verity
   team) adds it. To avoid losing Go CVE coverage for the 16+
   `goVcsUrl`-tagged images in `copa-config.yaml` during the migration
   window, `go.mod` uses a temporary `replace` directive pointing at
   `verity-org/copacetic feat/go-vcs-resolution` (upstream main + PR #1546).
   `--go-vcs-url` is fully wired: CLI flag → `Config.GoVCSURL` →
   `types.Options.GoVCSURL`. `TestRunPlumbsGoVCSURLToCopa` locks the
   plumbing in. When PR #1546 merges upstream, drop the replace directive.

4. **BuildKit connection is internal to copa** (plan Risk R3 resolved). Pass
   `BkAddr` string, copa calls `buildkit.NewClient` itself. No manual
   `buildkit.Client` wiring required. `"tcp://127.0.0.1:1234"` works locally
   against `moby/buildkit:v0.29.0` from our `docker-compose.yaml`.

5. **`ErrNoUpdatesFound` re-export.** We re-export `types.ErrNoUpdatesFound`
   as `patch.ErrNoUpdatesFound` so `cmd/patch.go` can call
   `errors.Is(err, patch.ErrNoUpdatesFound)` without pulling in copa types.
   Needed because patch-image.sh's retry logic keys off the stderr string
   `"no package updates found for image"`.

6. **Copa renders a "Patch Failed" TUI box for ALL errors including
   `ErrNoUpdatesFound`.** Upstream `pkg/patch/patch.go` calls
   `tui.RenderError(getErrorInfo(err))` unconditionally on any non-nil
   return, including the sentinel. Verity swallows `ErrNoUpdatesFound` and
   exits 0, but the red "Patch Failed" panel has already been printed to
   stderr by the time we see the error. Low severity: exit code is correct,
   the grep-compatible stderr line still appears, and the CI retry gate
   works. Fixing upstream would require either intercepting copa's stderr or
   a `SuppressTUIOnNoUpdates` option; deferring.

## Fork Parity Verification (Review #5)

Reviewer asked whether the `verity-org/copacetic` fork carried unupstreamed
logic that would disappear with this migration. The fork's 68 commits above
upstream fell into three buckets (established in the prior copa-side session):

| Fork feature | Exposed to verity? | Status in migration |
|---|---|---|
| Go VCS fallback (PR #1546) | Yes — `--go-vcs-url` flag | **Fully wired** via a temporary `go.mod` replace directive → `verity-org/copacetic feat/go-vcs-resolution` (upstream main + PR #1546 rebased on it). `--go-vcs-url` flows through to copa's `types.Options.GoVCSURL`. Replace directive is dropped once PR #1546 merges upstream; no other code changes needed then. |
| Helm chart patching (PR #1547) | **No.** | Verity's chart-gen command does helm wrapper charts in pure Go (`internal/chartgen`). Copa's helm patching (in-place) was never invoked from verity. Fork's branch is functionally irrelevant to verity. |
| Bulk engine extras: `--dry-run`, `--output-json`, `target.registry`, per-arch tag exclusion | **No.** | Verity's pipeline uses GH Actions matrix fan-out to call `./verity patch` per-image-per-platform (which wraps copa's single-image `Patch()` via the library), not copa's bulk engine. Skip-detection happens at verity's `scan` job layer (checks existing patched image vulns, only dispatches patch if needed). None of the fork's bulk extras are invoked from verity's workflows. |

Evidence that bulk engine is not used:
- `.github/workflows/patch-image.yaml` fans out a matrix job per image+tag, each calling `patch-image.sh` which invokes `./verity patch` (single image, wrapping copa's `pkg/patch.Patch`).
- No verity script or workflow invokes copa's bulk engine, `upgrade-report`, or any bulk-config mode.
- Skip detection lives in `cmd/scan.go` (patched-image scan, emits `needs-patch=true/false` workflow output consumed by `patch-image.yaml:scan` job).

Conclusion: the only fork feature with a functional verity surface is the Go VCS fallback. Both the no-op acceptance of `--go-vcs-url` and the OS-only retry fallback preserve the existing behavior envelope. The migration does not drop any live functionality.

## End-to-End QA Scenario Result (Plan §6 Phase 1)

- Started `docker compose up -d` — registry on `:5555`, BuildKit on `:1234`.
- Built verity with copa import: `go build -o verity .` — clean.
- `go vet ./...` — clean.
- `go test ./...` — all packages pass (14 suites).
- `./verity patch --help` — 11 required flags all present; help text renders
  cleanly.
- End-to-end: `./verity patch --image docker.io/library/alpine:3.18.0 --tag alpine-smoke --report /tmp/empty-trivy.json --pkg-types os --buildkit-addr tcp://127.0.0.1:1234 --timeout 120s`
  - Output: `Patching: linux/amd64 -> docker.io/library/alpine:alpine-smoke` 
  - Then: `no package updates found for image docker.io/library/alpine:3.18.0` (stderr)
  - Exit code: `0` (correct — no updates is not an error)
- Validation path: `./verity patch --image ""` → `invalid patch config: image reference is required` → non-zero exit.

All Phase 1 QA gates green. Ready to proceed to Phase 3 (CI switchover).

## Deferred

- `--go-vcs-url` → `opts.GoVCSURL` wiring: pending upstream PR #1546 merge.
- Pin bump to tagged release: plan Phase 5, after PR #1546 lands and a new
  copa release ships.
- Retry-in-Go (plan Phase 4): keeping shell retry wrapper for now.
