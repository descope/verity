# Technical Architecture — Chart-Integration Test Harness

This document is the technical contract surface for the chart-integration
smoke harness and the chartgen pieces that feed it. It complements (does
not replace) the higher-level map in [`ARCHITECTURE.md`](../../ARCHITECTURE.md).
Each section here describes a mechanism that is enforced in code; if the
documentation and the code disagree, the code wins and this doc must be
updated.

Surfaces covered:

1. `test/chart-integration/SKIPS.yaml` — institutional-debt skip register.
2. Harness retry wrapper — `InstallChartWithRetry` and its classifier.
3. `chartgen` list/map `chartValues` support.
4. Image entrypoint conventions for `images/<chart>.yaml`.

See SCR-2026-05-14-001 (`docs/scrs/SCR-2026-05-14-001-chart-integration-recovery.md`)
for the change proposal that introduced these mechanisms; this document is
the steady-state contract.

---

## 1. `test/chart-integration/SKIPS.yaml`

`SKIPS.yaml` is the **institutional-debt register** for charts that cannot
pass the chart-integration smoke matrix. It is the *only* sanctioned path
for opting a chart out of the smoke suite — there is no `t.Skip` shortcut,
no harness-side allowlist, and no environment flag that bypasses it.

Materializes SCR-2026-05-14-001 §2 AC-3 (per-chart taxonomy as a tracked
file with linked issue + exit criterion).

### 1.1 Schema

Each entry is a YAML map under the top-level `skips:` list. **All six
fields are required.** The loader (`test/chart-integration/skips.go`,
`LoadSkips`) rejects entries missing any field.

| Field           | Type     | Constraint                                                                                       |
| --------------- | -------- | ------------------------------------------------------------------------------------------------ |
| `chart`         | string   | Chart name as it appears in `Chart.yaml dependencies[].name`. Must be safe — no `/`, `\`, `..`, whitespace, newlines. Must be unique across the file. |
| `reason`        | string   | One-sentence human-readable failure summary. Free text.                                          |
| `tracking_issue`| string   | Either a `github.com` `http(s)://` URL, OR the literal sentinel string `"needs new issue"`.      |
| `exit_criteria` | string   | A concrete, testable condition under which this entry can be removed (e.g. *"upstream chart adds livenessProbe.initialDelaySeconds knob"*). |
| `added`         | string   | ISO-8601 date the entry was added.                                                               |
| `added_by`      | string   | GitHub handle or specialist role that added it (for accountability and grep-ability).            |

### 1.2 Hard cap

```go
const MaxSkippedCharts = 5
```

(`test/chart-integration/skips.go`.) The cap is the institutional debt
budget approved in **SCR-2026-05-14-001 §2 AC-1**. Raising it requires a
new SCR — it is not a tuning knob. The loader fails closed if a sixth
entry is added; the chart-integration job exits non-zero before any chart
is even scheduled.

Rationale: a smoke suite that quietly grows skips is worse than a red
build, because it normalizes red-as-green. The cap forces every
incremental skip to compete with an existing entry whose exit criterion
must be re-examined.

### 1.3 Fail-closed invariants

`skips.go` validates eight rejection conditions. Any of these causes
`LoadSkips` to return an error, which `TestMain` converts to a fatal
harness exit:

1. **Malformed YAML.** Parse error → fail.
2. **Unknown top-level field.** Decoder runs with `KnownFields(true)`;
   typos in keys (e.g. `skipss:`) fail rather than silently no-op.
3. **Duplicate chart name.** Two entries for the same `chart:` fail.
4. **Cap exceeded.** `len(Skips) > MaxSkippedCharts` fails.
5. **Missing required field.** Any of the six fields empty/absent fails.
6. **Unsafe chart name.** Contains `/`, `\`, `..`, whitespace, or
   newline → fails (defense-in-depth against path-traversal/log-injection
   if the chart name ever lands in a shell argument or file path).
7. **Bad `tracking_issue` shape.** Not the `"needs new issue"` sentinel
   AND not a `github.com` `http(s)://` URL → fails. (Scheme-less URLs,
   non-GitHub trackers, and free-text are all rejected.)
8. **Surface invariant.** `TestProductionSKIPSYAMLIsValid` runs in CI on
   the on-disk `SKIPS.yaml` itself; a malformed live file fails the
   chart-integration job, not just a hypothetical test fixture.

Test coverage: `test/chart-integration/skips_test.go` — 9 test functions,
13 subtests; all pass under `VERITY_IT_SKIP_CLUSTER=1 go test
-tags=integration -run 'TestLoadSkips|TestIsSkipped|TestProductionSKIPSYAMLIsValid'
./test/chart-integration/...`.

### 1.4 Sentinel-file mechanism for step-summary rendering

Go's `t.Skipf` lets `make chart-integration` exit `0` whether a chart was
genuinely green or quietly skipped. GitHub Actions'
`steps.smoke.outcome` would then classify a skipped shard as `success`,
which would defeat the entire visibility goal of the skip register.

To break that ambiguity, the harness writes a sentinel file
`_skip-<chart>.txt` at the repo root immediately before calling
`t.Skipf`. The `Record shard outcome` step in
`.github/workflows/chart-integration.yaml` reads the sentinel and emits a
**three-way step-summary**:

- ✅ success — exit 0, no sentinel
- ⚠️ skipped — exit 0, sentinel present (renders chart name + tracking
  issue + exit criterion from `SKIPS.yaml`)
- ❌ failure — exit non-zero

Skips are thus visible at PR review time and at the nightly-summary
level, distinct from green shards. The sentinel is per-shard
(filename-scoped to the chart) so parallel shards do not race.

### 1.5 Lifecycle

**Adding a skip:**

1. Confirm the failure mode cannot be fixed by image-layer changes
   (Bucket A/B/H), chartValues tuning (Bucket E with `chartValues:`
   list/map support — §3 below), or harness retry (§2 below). The skip
   register is the **last resort**, not the first.
2. Open a GitHub issue describing the upstream blocker and the
   exit-criteria condition. (If no issue can be opened immediately, use
   the `"needs new issue"` sentinel and file the issue within the same
   PR cycle. The audit script greps for this sentinel.)
3. Add the entry to `SKIPS.yaml` with all six fields populated. If the
   cap is at 5 of 5, you must first remove an existing entry whose exit
   criterion has been met, or land a new SCR raising the cap.
4. The PR carrying the new entry must reference the SCR that authorized
   the original cap (SCR-2026-05-14-001) and the new tracking issue.

**Removing a skip:**

1. Verify the exit criterion is met (the underlying upstream change has
   landed, the image fix has shipped, etc.).
2. Delete the entry from `SKIPS.yaml`.
3. The next nightly proves green — or, if not, the chart re-enters the
   failure taxonomy and a new failure mode must be classified before the
   entry is re-added.

### 1.6 Cross-references

- Header comment in `test/chart-integration/SKIPS.yaml` itself
  (authoritative inline contract — read first).
- SCR-2026-05-14-001 §2 AC-1 (cap), §2 AC-3 (taxonomy materialization).
- Failure taxonomy buckets — `evidences/SCR-2026-05-14-001-chart-integration-recovery/logs/failure-taxonomy.md`
  Buckets F (kernel-incompat — falco) and G (chart-template-blocked —
  nfs-subdir, cilium, cert-manager-csi-driver, workload-identity-webhook)
  are the canonical skip-eligible failure modes. (Local-only;
  `evidences/` is `.gitignore`d.)

---

## 2. Harness retry wrapper

### 2.1 `InstallChartWithRetry`

Defined in `test/chart-integration/harness_retry.go`. Wraps the existing
`InstallChart` call with the following policy:

- **`MaxAttempts = 3`** (1 initial + 2 retries).
- **`Backoff = 30 * time.Second`** between attempts.
- Between attempts, the wrapper calls the existing `UninstallChart`
  cleanup path — no bespoke teardown. The next attempt is a clean
  `helm install` against an empty namespace, not a `helm upgrade`.
- Retries only on **pull-class** failures (network/registry transients).
  **Crash-class** and **unknown** failures fail fast.

Call-site policy: `main_test.go` continues to call plain `InstallChart`
in this commit; flipping the call site is a follow-up once subtask 8 has
validated that pull-class is the dominant remaining failure mode. The
wrapper is a **pure addition** — opting in is intentional, not implicit.

Materializes SCR-2026-05-14-001 §2 AC-4.

### 2.2 Classifier — `classifyHelmFailure`

A pure function over `(helmStderr string, podStatusSnapshot)` returning
one of three classes:

- `pull-class` → retry
- `crash-class` → fail-fast
- `unknown` → fail-fast (conservative)

Empty input → `unknown`.

### 2.3 Authoritative source-of-truth: the package-level needle/reason vars

The classifier is **data-driven**. Three package-level `[]string` vars in
`harness_retry.go` are the single source of truth:

| Var                    | Source              | Class on match                   |
| ---------------------- | ------------------- | -------------------------------- |
| `pullStderrNeedles`    | helm combined-output substrings (case-insensitive) | `pull-class` |
| `pullWaitingReasons`   | pod `containerStatuses[].state.waiting.reason` values | `pull-class` |
| `crashWaitingReasons`  | pod `containerStatuses[].state.waiting.reason` values | `crash-class` |

Anyone extending the classifier — adding a new pull-class needle, a new
crash signal, etc. — **edits these three slices**, not the classifier
function body. The function body is intentionally a thin pattern-match
over the slices; behavior is configured by data, not by code.

Current contents (commit-of-record `test/chart-integration/harness_retry.go`):

- `pullStderrNeedles` (10): `ErrImagePull`, `ImagePullBackOff`,
  `manifest unknown`, `failed to pull and unpack`,
  `failed to resolve reference`, `connection refused`, `i/o timeout`,
  `no such host`, `dial tcp`, `TLS handshake timeout`, `502`, `503`,
  `504` (counted as one bucket of HTTP-5xx needles).
- `pullWaitingReasons` (3): `ErrImagePull`, `ImagePullBackOff`,
  `RegistryUnavailable`.
- `crashWaitingReasons` (5): `CrashLoopBackOff`, `RunContainerError`,
  `CreateContainerError`, `CreateContainerConfigError`, `StartError`,
  plus the implicit "container `lastState.terminated.exitCode != 0`"
  signal (encoded in the classifier body, not the slice — it is a
  structural check, not a string match).

### 2.4 Crash-precedence rule

If a pod's container statuses present **both** a pull-class waiting
reason **and** a crash-class signal (different containers, or the same
container in different states across a sampling window), **crash wins**.
The classifier sweeps for crash signals first and returns immediately on
the first crash hit; only when no crash signal is present anywhere in
the namespace's pod statuses does it then look for pull signals.

Rationale: a `CrashLoopBackOff` on container B does not become "maybe
transient" because container A is `ImagePullBackOff` — the workload is
demonstrably broken at the application layer, and 30 seconds of waiting
will not fix it. Retrying would burn the retry budget on a guaranteed-red
attempt and delay the inevitable failure.

Test coverage: `test/chart-integration/harness_retry_test.go`
`TestClassifyHelmFailure` includes the mixed-pod and mixed-container
sub-cases that exercise this rule. 21 sub-tests total, all pass.

### 2.5 `parsePodStatusJSON` — dependency-free by design

`parsePodStatusJSON` parses a *subset* of `kubectl get pods -o json`
output into the local `podStatusSnapshot` struct. It deliberately does
**not** import `k8s.io/api/core/v1` or any other `k8s.io` package.

Two reasons:

1. **Build-graph hygiene.** Pulling `k8s.io/api` into the test harness
   would pull a large transitive dependency closure (apimachinery,
   client-go, etc.) for the trivial purpose of unmarshaling four field
   paths (`.items[].status.containerStatuses[].state.waiting.reason`,
   `.lastState.terminated.exitCode`, plus name/kind framing). The
   harness already shells out to `kubectl` for everything else; the
   JSON it reads is a stable, well-known shape.
2. **Robustness to upstream churn.** The classifier reads four leaf
   field paths. A locally-defined struct that ignores unknown JSON keys
   is more robust to K8s API additions than tracking the full
   `corev1.PodStatus` shape across versions.

Test coverage: `TestClassifyHelmFailureParsesPodJSON` and
`TestClassifyHelmFailureMalformedJSON` exercise the happy path and
robustness against partial/truncated `kubectl` output.

---

## 3. `chartgen` list/map `chartValues` support

`verity.yaml` top-level `chartValues:` accepts **dotted-path keys with
terminal scalar, list, or map values**. The schema is a flat dotted-path
namespace; the terminal value at each key may be a scalar
(`string`/`bool`/number), a list, or a string-keyed map. Internal map
shapes, list-of-maps, and arbitrary nesting under a dotted-path key all
flow through to the rendered chart's `values.yaml`. Bracket-index syntax
(e.g. `foo[0].bar`) is not used — write the terminal value as a real
list/map instead.

Implementation lives in `internal/discovery/charts.go` and emits one
helm flag per `chartValues` entry: scalars go through `--set` /
`--set-string`; non-scalar terminals are JSON-encoded and forwarded via
`--set-json` (helm parses the JSON and merges it into the chart values
tree, preserving list and map shapes). Type detection uses `reflect` to
classify the terminal value — slices, arrays, and string-keyed maps
take the `--set-json` route; non-encodable types
(channels, functions, complex numbers, etc.) are rejected at config-load
time with `ErrChartValueUnsupportedType`. Per-pair `--set-json` is the
canonical mechanism (landed in
PR [#342](https://github.com/verity-org/verity/pull/342)); the image-override
precedence rule from
PR [#361](https://github.com/verity-org/verity/pull/361) is preserved
unchanged — chartValues cannot mask the patched/Integer image routing.

---

## 4. Image entrypoint conventions for `images/<chart>.yaml`

### 4.1 Background

`images/<chart>.yaml` is the integer/apko build config for a Wolfi-rebuilt
image. It may declare an `entrypoint:` field that lands in the published
OCI image manifest as `Entrypoint: [...]`. Combined with the Helm chart's
container `command:` and `args:`, runc decides what argv to exec at
container start.

The interaction is subtle, and three of the failure modes in the SCR
(argo-cd Bucket A, dex Bucket A, opensearch-dashboards Bucket B) were
caused by an over-specified `entrypoint:` field colliding with the
chart's hardcoded `args:`. This section captures the rule that emerged
from subtask 4b's investigation.

### 4.2 When to set `entrypoint:`

Set `entrypoint:` in `images/<chart>.yaml` when the image ships a
**single-purpose binary** that the chart's `command:` or `args:` invokes
by absolute path or relies on the image to provide as the default exec
target. Most simple-binary charts fall here (e.g. `prometheus`,
`grafana`, `etcd`).

### 4.3 When to OMIT `entrypoint:`

Omit `entrypoint:` (and the apko-emitted OCI `Entrypoint` is `null`) in
these three cases:

1. **Multicall binaries that dispatch on `argv[0]`.** The canonical
   example is argo-cd: a single `/usr/local/bin/argocd` binary plus a set
   of symlinks (`argocd-server`, `argocd-repo-server`, `argocd-application-controller`,
   `argocd-applicationset-controller`, `argocd-notifications`,
   `argocd-cmp-server`, `argocd-dex`, `argocd-git-ask-pass`,
   `argocd-k8s-auth`). The binary's `cmd/main.go` does
   `filepath.Base(os.Args[0])` and switches into the matching sub-command.
   If `entrypoint: /usr/local/bin/argocd` is set, runc execs *that path*
   regardless of which symlink the chart's `args:` named — argv[0]
   becomes `argocd` and the dispatch falls to the default branch.
2. **Bare-name chart args.** The canonical example is dex: the upstream
   Helm chart hardcodes `args: [dex, serve, /etc/dex/config.yaml]`. When
   image `Entrypoint` is `null`, runc passes that as full argv,
   PATH-resolves the bare `dex` to `/usr/bin/dex`, and argv[0] stays
   `"dex"` for cobra's `rootCmd.Use: "dex"` to match. If
   `entrypoint: /usr/bin/dex` is set, the chart's `dex` argv-element
   becomes argv[1] and cobra rejects it as an unknown subcommand
   ("`unknown command "dex" for "dex"`").
3. **Charts whose Helm template hardcodes `args:` with no `command:`
   override.** The opensearch-dashboards Bucket B mode: chart calls the
   upstream Bitnami entrypoint script via `args:` and expects the image
   to default to invoking it. An over-specified `entrypoint:` in the
   rebuild collides with the chart's args and the wrong argv lands.

### 4.4 The contract

When `entrypoint:` is **omitted** from `images/<chart>.yaml`:

1. apko emits the OCI image manifest with **`Entrypoint: null` AND
   `Cmd: null`**.
2. Kubernetes' container runtime sees no image-level exec defaults and
   passes the chart's container `args:` (or `command:` + `args:` if the
   chart sets both) **as the full argv** to runc.
3. runc resolves `argv[0]`:
   - **Absolute path** (e.g. `/usr/local/bin/argocd-server`) → direct
     `execve(2)` through the file. Symlinks are resolved by the kernel;
     **argv[0] is preserved verbatim** through the symlink walk (this is
     what makes argv[0]-dispatch work).
   - **Bare name** (e.g. `dex`) → PATH-resolution via the image's `PATH`
     env. **argv[0] is preserved verbatim** as the bare name (this is
     what lets cobra's `Use: "dex"` match).

The crucial property — verified for both argo-cd and dex against
upstream source — is **argv[0] preservation** through both resolution
paths. The kernel does not rewrite argv[0] when walking symlinks; runc
does not rewrite argv[0] during PATH-resolution. So the chart's args[0]
ends up at the process boundary as-is, and the application's argv[0]
inspection (`filepath.Base(os.Args[0])` for argo-cd; cobra `Use` match
for dex) works correctly.

### 4.5 Verification

Subtask 4b verified argv[0] preservation against upstream sources:

- **argo-cd** — `github.com/argoproj/argo-cd` `cmd/main.go` switches on
  `filepath.Base(os.Args[0])`. Also accepts `ARGOCD_BINARY_NAME` env
  override.
- **dex** — `github.com/dexidp/dex` `cmd/dex/main.go` is a plain cobra
  `rootCmd` with `Use: "dex"`, no `os.Args[0]` inspection (so for dex,
  argv[0] only needs to be present, not specifically inspected).

Crane-config inspection of published images is the steady-state check —
look for `Entrypoint: null + Cmd: null` in the OCI config when the
chart's `args:` is supposed to land as full argv.

### 4.6 Cross-references

- `images/argocd.yaml` (default + fips variants), `images/dex.yaml` —
  reference fixes that landed under this convention.
- `internal/integer/render/render.go` — the `render.Config` that emits
  the apko YAML faithfully omits the `entrypoint:` key when
  `TypeTemplate.Entrypoint == ""`. (Verified inline by an ephemeral
  probe test during subtask 4b; not committed.)
- SCR-2026-05-14-001 §2 AC-5 — "Bucket A is fixed at the image layer for
  at least argo-cd + dex (proof: container starts without 'unknown
  command' error on local kind run)".
