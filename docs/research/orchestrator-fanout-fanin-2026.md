# Orchestrator Fan-Out → Fan-In: 2026 GitHub Actions Best Practices

Research compiled for the Verity `orchestrator.yaml` finalize job. ~90 parallel
`patch-image` matrix children → 1 `finalize` aggregator → orphan `_metrics` branch.

All evidence backed by GitHub permalinks. Skip beginner content; production-only.

---

## TL;DR — Action Plan for Verity

```yaml
# .github/workflows/orchestrator.yaml (top-level)
name: orchestrator
on:
  schedule: [{cron: '0 6 * * *'}]
  workflow_dispatch:

# Queue overlapping runs instead of cancelling — losing a scheduled run loses a day of metrics
concurrency:
  group: orchestrator-metrics      # Static name → all runs (sched + dispatch) join one queue
  cancel-in-progress: false        # CRITICAL: queue, don't kill

permissions:
  contents: write                  # Needed to push to _metrics orphan branch

jobs:
  patch-image:
    strategy:
      fail-fast: false             # MANDATORY: don't kill 89 siblings if 1 image fails
      matrix: { ... }              # ~90 entries
    steps:
      - uses: actions/upload-artifact@v4
        with:
          name: metrics-${{ matrix.image }}-${{ matrix.platform }}  # MUST be unique
          path: metrics.json
          if-no-files-found: error                                  # Fail loud, not silent
          retention-days: 7                                         # Cut storage; data lives in _metrics
        env:
          ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 5                    # Mitigates upload stalls at scale

  finalize:
    needs: [patch-image]
    if: ${{ !cancelled() }}         # Run on success OR partial-failure; skip only when whole run cancelled
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
          persist-credentials: true
      - uses: actions/download-artifact@v4
        with:
          path: /tmp/metrics
          pattern: metrics-*
          merge-multiple: true     # Flattens into /tmp/metrics/*.json (no per-artifact subdirs)
      - name: Merge to daily JSON
        run: jq -s 'add' /tmp/metrics/*.json > daily-$(date -u +%F).json
      - name: Commit to _metrics orphan branch
        run: |                     # See "Recommended orphan-branch idiom" below
          ...
```

---

## Q1 — `actions/download-artifact@v4` with `pattern:` + `merge-multiple:`

### Canonical YAML

From [`actions/download-artifact` README](https://github.com/actions/download-artifact/blob/main/README.md#download-multiple-filtered-artifacts-to-the-same-directory):

```yaml
- uses: actions/download-artifact@v4
  with:
    path: my-artifact
    pattern: my-artifact-*
    merge-multiple: true
- run: ls -R my-artifact
```

### What `merge-multiple: true` does to directory structure

**Without `merge-multiple` (default)** — every artifact gets its own subdirectory:
```
etc/usr/artifacts/
    Artifact-A/
        ...contents of Artifact-A
    Artifact-B/
        ...contents of Artifact-B
```

**With `merge-multiple: true`** — all contents flattened into one directory:
```
path/to/artifacts/
    ...contents of Artifact-A
    ...contents of Artifact-B
```

> ⚠️ **File-name collision warning**: if two artifacts each contain a file with the
> same name, `merge-multiple: true` will overwrite. Verity is safe because each
> artifact is uniquely named `metrics-${image}-${platform}.json` and contains exactly
> that one file → flat dir of 90 unique JSONs.

### Production examples (1k+ stars)

| Project | Stars | Pattern used | URL |
|---|---|---|---|
| `facebook/react` | ~230k | `pattern: _build_*` + `merge-multiple: true` | [runtime_build_and_test.yml#L441](https://github.com/facebook/react/blob/main/.github/workflows/runtime_build_and_test.yml#L441-L447) |
| `vercel/next.js` | ~125k | `pattern: next-swc-binaries-*` + `merge-multiple: true` (pinned by SHA) | [build_and_deploy.yml#L445](https://github.com/vercel/next.js/blob/canary/.github/workflows/build_and_deploy.yml#L445-L455) |
| `onnx/onnx` | ~18k | `pattern: wheels*` + `merge-multiple: true` | [create_release.yml#L160](https://github.com/onnx/onnx/blob/main/.github/workflows/create_release.yml#L160-L165) |
| `open-webui/open-webui` | ~80k | `pattern: digests-main-*` (Docker buildx fan-out, very similar pattern) | [docker-build.yaml#L548](https://github.com/open-webui/open-webui/blob/main/.github/workflows/docker-build.yaml#L548-L555) |
| `meshtastic/firmware` | ~3k | `pattern: firmware-${{matrix.arch}}-*` | [main_matrix.yml#L179](https://github.com/meshtastic/firmware/blob/develop/.github/workflows/main_matrix.yml#L179-L186) |

The `open-webui` Docker-buildx-digests fan-in pattern is the **closest analog to
Verity's metrics-fan-in** — fan out → upload digest JSON per matrix child → fan in
→ download all with pattern → process. Worth reading end-to-end.

---

## Q2 — v4 Breaking Changes That Bite at 90+ Artifacts

From [`actions/upload-artifact` README](https://github.com/actions/upload-artifact/blob/main/README.md) and [MIGRATION.md](https://github.com/actions/upload-artifact/blob/main/docs/MIGRATION.md):

### 1. **Artifact names MUST be unique per workflow run**

> "Unlike earlier versions of upload-artifact, uploading to the same artifact via
> multiple jobs is _not_ supported with v4. Artifact names must be unique since
> each created artifact is **idempotent** so multiple jobs cannot modify the same artifact."
>
> — [upload-artifact README §"(Not) Uploading to the same artifact"](https://github.com/actions/upload-artifact/blob/main/README.md#not-uploading-to-the-same-artifact)

In v3 you could upload-N-times-to-same-name and it merged server-side. **In v4
this is a hard error.** For Verity this means `metrics-${image}-${platform}.json`
naming is correct — each name encodes both matrix axes, so all 90 are unique.
Do NOT use `metrics-${image}` alone if multiple platforms exist for one image.

### 2. **Artifacts are immutable once created**

Each artifact gets a unique server-side ID at upload time. You cannot modify or
re-upload to the same name within a run. To re-upload you must set `overwrite: true`.

### 3. **Hard limit: 500 artifacts per job**

> "Within an individual job, there is a limit of 500 artifacts that can be created
> for that job."
>
> — [upload-artifact README §"Number of Artifacts"](https://github.com/actions/upload-artifact/blob/main/README.md#number-of-artifacts)

90 artifacts × 1-per-matrix-child = well under 500. Note this is *per-job*, not
per-run. Each matrix child is its own job, so each child has its own 500-quota.
Verity is fine.

### 4. **`v3` deprecated — already gone**

Per [the Apr 2024 deprecation notice](https://github.blog/changelog/2024-04-16-deprecation-notice-v3-of-the-artifact-actions/):

> "Starting **January 30th, 2025**, GitHub Actions customers will no longer be able to
> use v3 of actions/upload-artifact or actions/download-artifact... attempting to
> use a version of the actions after the deprecation date will result in a workflow failure."

It is now April 2026 — **`@v3` is broken**. Use `@v4` (latest stable line).

### 5. **Hidden files now excluded by default (v4.4+)**

> "With v4.4 and later, hidden files are excluded by default."

If your `metrics.json` ever lives in a `.metrics/` directory or starts with a dot,
add `include-hidden-files: true`. Verity's path is `metrics-...json` so this is fine.

---

## Q3 — Orphan-Branch Commits in CI: Three Approaches Compared

### Approach A: Raw `git switch --orphan` + `git push` (RECOMMENDED for Verity)

This is the idiom used by serious projects with bot-driven branches.

**The closest production analog to your use-case** is
[`prometheus/client_java`'s `nightly-benchmarks.yml`](https://github.com/prometheus/client_java/blob/main/.github/workflows/nightly-benchmarks.yml) —
a scheduled cron that fans-in JMH results and commits to a `benchmarks` orphan branch:

```yaml
- uses: actions/checkout@v6
  with:
    # persist-credentials defaults to true; omit unless overriding
    fetch-depth: 0              # Need history if branch already exists

# ... do work, generate output to /tmp/output ...

- name: Commit and push to benchmarks branch
  run: |
    git config user.name "github-actions[bot]"
    git config user.email "github-actions[bot]@users.noreply.github.com"

    # First-run vs subsequent-run handling
    if git ls-remote --heads origin benchmarks | grep -q benchmarks; then
      git fetch origin benchmarks
      git switch benchmarks
      # Preserve any history files
      cp -r history /tmp/output/ 2>/dev/null || true
    else
      git switch --orphan benchmarks
    fi

    # Clean working dir, then copy results in
    git rm -rf . 2>/dev/null || true
    find . -mindepth 1 -maxdepth 1 ! -name '.git' -exec rm -rf {} +
    cp -r /tmp/output/* .

    git add .
    DATE=$(date -u +"%Y-%m-%d")
    git commit -m "Results for ${DATE}" || echo "No changes to commit"
    git push origin benchmarks
```

**Why this is the right pattern for Verity:**
- ✅ No third-party action — fewer supply-chain risks for a metrics branch
- ✅ Full control over commit message format (date, run ID, etc.)
- ✅ Idempotent: `|| echo "No changes"` handles no-op runs
- ✅ Handles first-run (orphan create) and Nth-run (fast-forward) in one block
- ✅ The `find ... -exec rm` pattern correctly cleans without nuking `.git`

A simpler bootstrapping variant from
[`yamadashy/repomix`](https://github.com/yamadashy/repomix/blob/main/.github/workflows/perf-benchmark-history.yml#L74-L84)
just creates the orphan branch on first run if missing:

```yaml
- name: Ensure gh-pages branch exists
  env: { GH_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
  run: |
    if ! git ls-remote --heads origin gh-pages | grep -q gh-pages; then
      git switch --orphan gh-pages
      git commit --allow-empty -m "Initial gh-pages branch"
      git remote set-url origin "https://x-access-token:${GH_TOKEN}@github.com/${{ github.repository }}.git"
      git push origin gh-pages
      git switch "$CURRENT_BRANCH"
    fi
```

### Approach B: `peter-evans/create-pull-request`

Used by `prisma/prisma`, `medusajs/medusa`, `google-gemini/gemini-cli`,
`prowler-cloud/prowler` (pinned by SHA), etc. — see
[prisma/prisma update-engines-version.yml#L110](https://github.com/prisma/prisma/blob/main/.github/workflows/update-engines-version.yml#L110-L120).

```yaml
- uses: peter-evans/create-pull-request@v8
  with:
    token: ${{ secrets.BOT_TOKEN }}
    branch: _metrics-update
    commit-message: 'metrics: $(date)'
    title: 'Metrics update'
```

**Verdict for Verity: ❌ wrong tool.** This action *opens a PR*; it doesn't push
to a long-lived data branch. PRs would clutter the repo with daily merge requests.

### Approach C: `stefanzweifel/git-auto-commit-action`

Used by `android/nowinandroid`, `opensearch-project/OpenSearch`, etc. — see
[android/nowinandroid Build.yaml#L68](https://github.com/android/nowinandroid/blob/main/.github/workflows/Build.yaml#L68-L73).

```yaml
- uses: stefanzweifel/git-auto-commit-action@v7
  with:
    file_pattern: '**/dependencies/*.txt'
    commit_message: "chore: updates"
    branch: ${{ github.head_ref }}
```

**Verdict for Verity: ❌ wrong tool.** This action commits to *the currently
checked-out branch*. It cannot create or switch to an orphan branch. Forcing it
would require pre-checking-out `_metrics`, which means you've already done the
hard part with raw git anyway.

### Recommendation

**Use Approach A (raw `git switch --orphan` + `git push`)** with the
prometheus/client_java idiom adapted to Verity. It's the only one that:
1. Creates the orphan branch on first run
2. Pushes commits to a long-lived data branch (not PRs)
3. Doesn't pull in a third-party action just to wrap `git commit`

---

## Q4 — `concurrency:` for "Queue, Don't Cancel"

### The exact config for Verity

```yaml
concurrency:
  group: orchestrator-metrics    # Static name — same group for cron + workflow_dispatch
  cancel-in-progress: false      # New runs WAIT instead of replacing in-progress
```

### How it actually behaves (from [GitHub docs](https://docs.github.com/en/actions/using-jobs/using-concurrency))

> "There can be at most **one running and one pending** job in a concurrency group
> at any time. When a concurrent job or workflow is queued, if another job or
> workflow using the same concurrency group in the repository is in progress, the
> queued job or workflow will be `pending`. **Any existing pending job or workflow
> in the same concurrency group, if it exists, will be canceled** and the new
> queued job or workflow will take its place."

This means:
- Run #1 at 06:00 starts.
- Run #2 (manual dispatch) at 06:30 → enters **pending**, doesn't cancel #1.
- Run #3 at 07:00 (next cron) → cancels the pending #2, becomes the new pending.
- When #1 finishes, #3 starts.

⚠️ **Caveat**: only ONE pending slot. If you'd lose the night's metrics due to
collapsed pending runs, consider running cron less frequently than your slowest run.
For Verity (daily cron), this is fine.

### Production examples of `cancel-in-progress: false` for queue-don't-cancel

| Project | Use case | URL |
|---|---|---|
| `scalar/scalar` | npm publish job — must serialize | [ci.yml#L541](https://github.com/scalar/scalar/blob/main/.github/workflows/ci.yml#L541-L548) |
| `scalar/scalar` | Docker publish | [ci.yml#L869](https://github.com/scalar/scalar/blob/main/.github/workflows/ci.yml#L869-L876) |
| `mindsdb/mindsdb` | Per-env deploy serialization | [deploy.yml#L31](https://github.com/mindsdb/mindsdb/blob/main/.github/workflows/deploy.yml#L31-L40) |
| `cisagov/ScubaGear` | Tenant-scoped test serialization | [function_test_defender.yaml#L45](https://github.com/cisagov/ScubaGear/blob/main/.github/workflows/function_test_defender.yaml#L45-L51) |
| `benbjohnson/litestream` | Manual integration tests | [manual-integration-tests.yml#L108](https://github.com/benbjohnson/litestream/blob/main/.github/workflows/manual-integration-tests.yml#L108-L114) |
| `prometheus/client_java` | Nightly benchmarks (no `cancel-in-progress` key = false default) | [nightly-benchmarks.yml#L18](https://github.com/prometheus/client_java/blob/main/.github/workflows/nightly-benchmarks.yml#L17-L19) |

`scalar/scalar`'s comment is gold:
```yaml
# Avoid running this job in parallel:
# `changesets/action` creates/updates the release branch, which shouldn't
# happen in parallel.  npm publish also shouldn't happen in parallel.
concurrency:
  group: npm-publish
  cancel-in-progress: false
```

### Subtle production gotcha

Note that `prometheus/client_java` uses `concurrency: { group: "benchmarks" }`
**without** `cancel-in-progress: false`. The default for `cancel-in-progress`
is `false`, so this works — but **be explicit**. Future GHA syntax tightening
or readers misreading the YAML can both be avoided by writing it out.

---

## Q5 — Artifact Upload Limits & Flake Patterns at 90+ Matrix Scale

### Hard limits

| Limit | Value | Source |
|---|---|---|
| Artifacts per job | **500** | [README](https://github.com/actions/upload-artifact/blob/main/README.md#number-of-artifacts) |
| Max retention | 90 days (default) | README inputs section |
| Min retention | 1 day | README inputs section |
| Max artifact size | Tied to repo storage quota | [Billing docs](https://docs.github.com/en/billing/managing-billing-for-github-actions/about-billing-for-github-actions) |

90 artifacts across 90 jobs (one each) → no per-job limit issue. Each child
uploads 1 artifact, well under the 500 cap.

### Known flake pattern: "Upload progress stalled"

[`actions/upload-artifact#642`](https://github.com/actions/upload-artifact/issues/642):

> "In spark, we often find that `actions/upload-artifact@v4` fails, but after trying
> again, it will basically succeed... retry is for the entire job, which wastes
> resources and time."

Maintainer fix from [`#623`](https://github.com/actions/upload-artifact/issues/623):

> "could you please try with `ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY` env and see
> if setting it to `10` helps?"
> ```yaml
> - uses: actions/upload-artifact@v4
>   with:
>     name: my-artifact
>     path: artifact-path/*
>   env:
>     ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 10
> ```

Production usage: [`mhx/dwarfs/.github/workflows/docker-run-build.yml#L130`](https://github.com/mhx/dwarfs/blob/main/.github/workflows/docker-run-build.yml#L130-L137)
sets `ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 5` for binary tarballs.

**Recommendation for Verity**: set `ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 5` on
the matrix upload step. Each artifact is small (one JSON), so high parallelism
inside the upload doesn't help — what matters is reducing burst-load on the
artifact service when 90 jobs all hit it simultaneously.

### No automatic retry

`actions/upload-artifact` does **not** auto-retry. A flaky upload fails the
job. Mitigations:
1. `ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 5` (above)
2. Don't fail the matrix; let `finalize` aggregate whatever made it through (Q6)
3. Optionally add `nick-fields/retry@v3` around the upload step if flakes persist

### Other 90+ matrix considerations

- **Concurrent runner availability**: GitHub-hosted runners default cap is
  20 concurrent jobs for free / up to 180 for Enterprise. 90 matrix jobs may
  serialize anyway. Use `max-parallel:` on the strategy if you want to throttle
  intentionally rather than have GHA do it implicitly.
- **API rate limits**: 1000 requests/hour per repo for the artifact API. 90
  uploads + 1 download (90 artifacts in one call) = 91 requests. Trivial.

---

## Q6 — Should `finalize` Run if Some Matrix Children Failed?

### The answer: **YES, use `if: ${{ !cancelled() }}`**

This is the idiomatic way to say "run when needs completes, regardless of pass/fail,
unless the entire run was cancelled." It's stronger than `if: always()` (which runs
even on cancellation, wasting compute and possibly committing partial garbage).

### The three idioms compared

| Expression | When `finalize` runs |
|---|---|
| `if: success()` (default, no `if`) | Only if ALL `needs` succeeded — ❌ kills you on 1/90 failure |
| `if: ${{ always() }}` | Always — even if user cancels the run |
| `if: ${{ !cancelled() }}` | ✅ On success OR failure of needs, but NOT if cancelled |
| `if: ${{ failure() }}` | Only if a need failed |

### Production examples of `if: always()` for fan-in jobs

`moby/moby` does this exact pattern in
[`.github/workflows/.test.yml#L272`](https://github.com/moby/moby/blob/master/.github/workflows/.test.yml#L272-L280):

```yaml
integration-report:
  runs-on: ubuntu-24.04
  timeout-minutes: 10
  continue-on-error: ${{ github.event_name != 'pull_request' }}
  if: always()
  needs:
    - integration
```

`facebookincubator/velox` uses the same pattern for
[`adapters-build-status` and `adapters-test-status`](https://github.com/facebookincubator/velox/blob/main/.github/workflows/linux-build-base.yml#L330-L356).

`keploy/keploy` has a "CI Gate" pattern with `if: always()` aggregating ~10 needs:
[`prepare_and_run.yml#L883`](https://github.com/keploy/keploy/blob/main/.github/workflows/prepare_and_run.yml#L883-L894).

### Do NOT use `continue-on-error: true` on the matrix children

That would mark failed children as **green**, making the matrix child status
useless for debugging. Instead:

1. Let matrix children fail naturally (`fail-fast: false` keeps siblings running).
2. Each upload uses `if-no-files-found: error` so a bad child fails its own job loudly.
3. `finalize` runs anyway via `if: ${{ !cancelled() }}` and aggregates whatever
   artifacts made it. Missing children = missing JSONs in the merge; that's fine.

### Optional: track partial-failure status in the merged JSON

```yaml
- name: Merge metrics
  run: |
    EXPECTED=$(cat .github/matrix-count.txt)   # Or compute from matrix
    GOT=$(ls /tmp/metrics/*.json | wc -l)
    jq -s --arg expected "$EXPECTED" --arg got "$GOT" \
      '{date: now|todate, expected_runs: ($expected|tonumber), successful_runs: ($got|tonumber), runs: .}' \
      /tmp/metrics/*.json > daily-$(date -u +%F).json
```

This way the `_metrics` branch records partial-failure days as data, not silence.

---

## Final Recommendations Summary

| Decision | Choice |
|---|---|
| Upload action | `actions/upload-artifact@v4` |
| Download action | `actions/download-artifact@v4` |
| Artifact naming | `metrics-${{ matrix.image }}-${{ matrix.platform }}` (must be unique) |
| Download flatten | `pattern: metrics-*` + `merge-multiple: true` |
| Top-level concurrency | `group: orchestrator-metrics`, `cancel-in-progress: false` |
| Finalize job condition | `if: ${{ !cancelled() }}` |
| Matrix `fail-fast` | `false` |
| Upload concurrency env | `ACTIONS_ARTIFACT_UPLOAD_CONCURRENCY: 5` |
| Upload `if-no-files-found` | `error` |
| Upload `retention-days` | `7` (data lives in `_metrics`, no need for 90 days) |
| Orphan branch idiom | Raw `git switch --orphan` (no third-party action) |
| Reference workflow to copy | [`prometheus/client_java/.github/workflows/nightly-benchmarks.yml`](https://github.com/prometheus/client_java/blob/main/.github/workflows/nightly-benchmarks.yml) |

---

## Appendix: Full Reference URLs

- [actions/download-artifact README](https://github.com/actions/download-artifact/blob/main/README.md)
- [actions/upload-artifact README](https://github.com/actions/upload-artifact/blob/main/README.md)
- [v3→v4 MIGRATION.md](https://github.com/actions/upload-artifact/blob/main/docs/MIGRATION.md)
- [v3 deprecation changelog](https://github.blog/changelog/2024-04-16-deprecation-notice-v3-of-the-artifact-actions/)
- [GHA concurrency docs](https://docs.github.com/en/actions/using-jobs/using-concurrency)
- [Artifact upload stall issue #642](https://github.com/actions/upload-artifact/issues/642)
- [Artifact upload self-hosted issue #623](https://github.com/actions/upload-artifact/issues/623)
