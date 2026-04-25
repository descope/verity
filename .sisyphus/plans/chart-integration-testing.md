# Chart Integration Smoke Test

## Goal

Catch broken wrapper charts every night, before users do. The nightly
`chart-gen` workflow publishes wrapper charts to
`oci://ghcr.io/verity-org/charts`; this workflow installs each one on a
kind cluster and verifies the published artifact actually deploys with
patched images.

This is a smoke test of **published artifacts** — not a rebuild, not a
reconstruction, not a unit test. We pull what was published and run it.

## What it Catches

| Failure class                                               | Signal                                  |
|-------------------------------------------------------------|-----------------------------------------|
| Wrapper chart never published / version mismatch            | `helm pull` fails                       |
| Patched image missing from `ghcr.io/verity-org`             | `ImagePullBackOff` -> `--wait` timeout  |
| Patched image broken (Copa corrupted binary, missing libc)  | `CrashLoopBackOff` -> never Ready       |
| Patched image starts but dies under load                    | restartCount > 0 in 30 s settle window  |
| `chart-gen` missed an image (subchart-resolution, split fmt)| image-origin assertion lists exact refs |
| Renovate bumped chart but patched tag not yet published     | image-origin or pull failure            |
| Wrapper chart manifest malformed                            | `helm install` fails at template render |

## What it Does NOT Catch (intentional)

- CVE coverage of the patches themselves -> existing trivy job in
  `pr-test.yaml` does this.
- Cosign signature validity, SBOM correctness -> separate verify steps.
- Operator behavior under real CRs -> install the operator pod only.
- Multi-arch -> amd64 only; arm64 covered by the publish pipeline.
- Production-scale issues -> single-node kind, no real load.

## Test Flow (per chart)

```text
1. Stand up kind v1.35 cluster.
2. helm install <chart> oci://ghcr.io/verity-org/charts/<chart>
       --version <pinned> --wait --timeout=10m
   (no local registry, no chart-gen invocation, no image seeding -
    we install exactly what was published.)
3. helm test <release>          (no-op if chart ships no test hooks)
4. sleep 30 s                   (settle window for delayed crashes)
5. assert: every container's restartCount == 0
6. assert: every running container image starts with
            ghcr.io/verity-org/  OR  is in the chart's optional
            allowlist file
7. helm uninstall + delete namespace
8. Tear down kind cluster.
```

The image-origin assertion is the canary: when chart-gen has a gap, the
published wrapper chart points some pods at upstream registries. The
assertion lists every leaking image with pod name + container name, so
the fix path is obvious.

## Adding a New Chart

1. Add the dep to `Chart.yaml`.
2. Wait for the next nightly `chart-gen` to publish the wrapper.
3. The smoke test workflow auto-discovers the new chart from `Chart.yaml`
   on its next run; no workflow edits needed.

If the chart cannot install on a vanilla kind cluster (needs cert-manager
pre-installed, requires a cloud LB, etc.), drop a values fixture at
`test/chart-integration/values/<chart>.yaml`. **Fixtures are optional.**
The default install path uses upstream defaults.

## Fixtures Convention (Optional)

- `values/<chart>.yaml` -- Helm values overlay scoped under the chart
  name. Used only when the chart can't install on kind defaults.
- `values/<chart>.allowlist.txt` -- one upstream image-ref prefix per
  line. The image-origin assertion accepts these in addition to
  `ghcr.io/verity-org/`. Lines starting with `#` and blank lines are
  comments. Add an entry only with a comment linking to a follow-up
  issue, never silently. Allowlist size is a code-review signal: it
  should shrink as chart-gen gaps close.

## CI Triggers

- **`workflow_run`** after `chart-gen.yaml` completes successfully on
  `main` -- the primary signal. Every nightly publish is smoke-tested
  immediately.
- **`schedule`** at `05:00 UTC` -- catches missed `workflow_run` events
  and acts as a safety net even if the publish workflow didn't trigger.
- **`pull_request`** with path filters on
  `test/chart-integration/**`, `.github/workflows/chart-integration.yaml`,
  `mise.toml` -- iterate the test framework itself.
- **`workflow_dispatch`** with optional single-chart input -- on-demand.

## Files Owned by This Plan

```text
test/chart-integration/
  main_test.go         # //go:build integration; runner; reads Chart.yaml
  harness.go           # //go:build integration; kind cluster lifecycle
  chart.go             # //go:build integration; helm install/uninstall
  assertions.go        # //go:build integration; image-origin + no-restart
  assertions_test.go   # //go:build integration; pure-Go unit tests
  kind.yaml            # vanilla kind cluster config (no registry mirror)
  values/.gitkeep      # documents the optional fixtures convention
.github/workflows/chart-integration.yaml   # dynamic matrix runner
```

Modified: `mise.toml` (kind+kubectl pins), `Makefile` (chart-integration
target), `.gitignore` (diagnostic dump dirs).

## Non-Goals

- Per-chart Go test files (one harness, period).
- In-test chart-gen invocation (we test artifacts, not rebuild them).
- Local image registry (we pull from the real `ghcr.io/verity-org`).
- Rewriting fixtures defensively for charts that already install
  cleanly (only add a fixture when an actual install fails).
- Multi-arch coverage.
- CVE / signature / SBOM verification (separate workflows).

## Success Criteria

- `make chart-integration` exits 0 for charts that install cleanly on
  vanilla kind, and reports actionable errors for charts that don't.
- New `chart-integration.yaml` runs after every successful `chart-gen`,
  matrix matches `Chart.yaml` deps automatically.
- Image-origin assertion fails any container image not under
  `ghcr.io/verity-org/` and not allowlisted, with pod/container/image
  details in the error message.
- `go test ./...` (no tag) is unchanged -- integration files are gated
  by `//go:build integration` and contribute zero statements.
- All existing CI (`lint.yaml`, `ci.yaml`, `pr-test.yaml`) stays green.

## Decisions Logged

1. **No local registry**: we test the published artifact, not a rebuild.
2. **No fixture-required gate**: every chart in `Chart.yaml` runs every
   night; failures are loud and informative.
3. **Allowlist semantics**: prefix match.
4. **Workflow trigger**: `workflow_run` after `chart-gen` success +
   `schedule` safety net + `pull_request` for test code + dispatch.
5. **Charts that need cluster setup beyond kind defaults**: out of scope
   for now. Add a fixture or a workflow-level pre-step (cert-manager
   etc.) only when the failure is actually observed.

## Surfaced Production Issues (caught during PR drafting)

While verifying the design works, helm-pulling the published charts
revealed three real bugs the smoke test would catch nightly:

- **postgres-operator wrapper @1.15.1**: ships a malformed image ref
  `ghcr.io/ghcr.io/verity-org/zalando/postgres-operator:v1.15.1` --
  chart-gen does not handle the `image.{registry,repository,tag}` split
  format. Pods would never pull. Caught by `helm install --wait` timeout.
- **prometheus wrapper @29.2.1**: 5 subchart images (alertmanager,
  prometheus-server, kube-state-metrics, node-exporter, pushgateway)
  are NOT rewritten and still point at upstream registries. chart-gen
  does not descend into subchart values. Caught by image-origin.
- **dex @0.24.0 and metrics-server @3.13.0**: not present in the
  published catalog at all. Either the nightly skipped them or
  publishing failed silently. Caught by `helm pull` failure.

These three are filed as chart-gen follow-ups; the smoke test is the
right place to detect future regressions of the same shape.
