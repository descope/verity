# Workflow Architecture

Verity uses a matrix-based workflow system to scan, patch, and publish security-hardened Helm charts in parallel across GitHub Actions runners.

## Overview

```
┌─────────────────┐
│   Chart.yaml    │  ← Renovate updates versions
│   values.yaml   │  ← Renovate updates image tags
└────────┬────────┘
         │
    ┌────▼────────────────────────────────────────────────────┐
    │  patch-matrix.yaml  /  scheduled-scan.yaml              │
    │                                                         │
    │  Job 1: Discover       Job 2: Patch (matrix)            │
    │  ┌──────────────┐      ┌───────────┐ ┌───────────┐     │
    │  │ Scan charts  │─────▶│ Image A   │ │ Image B   │ ... │
    │  │ Output matrix│      │ trivy+copa│ │ trivy+copa│     │
    │  └──────────────┘      └─────┬─────┘ └─────┬─────┘     │
    │                              │              │           │
    │  Job 3: Assemble       ┌─────▼──────────────▼─────┐    │
    │                        │ Collect results           │    │
    │                        │ Create wrapper charts     │    │
    │                        │ Commit / Create PR        │    │
    │                        └──────────────────────────┘    │
    └─────────────────────────────────────────────────────────┘
                       │
                  Merge to main
                       │
                       ▼
            ┌──────────────────────┐
            │  Publish to GHCR     │
            │  - Charts (OCI)      │
            │  - Images (Docker)   │
            └──────────────────────┘
```

## CLI Modes

Verity operates in distinct modes, each designed for a specific phase of the pipeline:

```bash
# Discover: scan charts, output GitHub Actions matrix
./verity -discover -chart Chart.yaml -images values.yaml -discover-dir .verity

# Patch single image: run in a matrix job
./verity -patch-single -image "quay.io/prometheus/prometheus:v3.9.1" \
  -registry ghcr.io/descope \
  -buildkit-addr docker-container://buildkitd \
  -report-dir .verity/reports \
  -result-dir .verity/results

# Assemble: create wrapper charts from matrix results
./verity -assemble \
  -manifest .verity/manifest.json \
  -results-dir .verity/results \
  -reports-dir .verity/reports \
  -output charts \
  -registry ghcr.io/descope

# Scan: list images without patching (dry run)
./verity -scan -chart Chart.yaml -images values.yaml

# Site data: generate catalog JSON from existing charts
./verity -site-data site/src/data/catalog.json -images values.yaml -registry ghcr.io/descope
```

## Workflows

### 1. CI Workflow (`ci.yaml`)

**Triggers:** Pull requests (except for `charts/**` and `**.md` changes)

**Purpose:** Validate code changes

**Jobs:**
- **Lint** - actionlint + shellcheck
- **Unit Tests** - Run Go tests
- **Scan** - Discover images in Chart.yaml dependencies (dry run)

---

### 2. Patch on PR (`patch-matrix.yaml`)

**Triggers:** Pull requests that modify `Chart.yaml` or `values.yaml`, or `workflow_dispatch`

**Purpose:** Automatically patch images when Renovate updates chart versions or image tags

**Jobs:**
1. **Discover** - Lightweight: scans charts, outputs matrix JSON and manifest
2. **Patch** (matrix) - Each image gets its own runner: pull → trivy → copa → push
3. **Assemble** - Collects results, creates wrapper charts, commits to PR

**Why matrix?** GitHub runners are small (2 vCPU, 7GB RAM). Pulling, scanning, and patching a container image is resource-intensive. Matrix jobs give each image a fresh runner with full resources and run in parallel.

---

### 3. Scheduled Scan (`scheduled-scan.yaml`)

**Triggers:**
- Cron: Daily at 2 AM UTC
- Manual: `workflow_dispatch`

**Purpose:** Continuously monitor for new vulnerabilities

**Jobs:** Same 3-job matrix pattern as `patch-matrix.yaml`, but the assemble step creates a PR instead of committing to an existing branch.

---

### 4. Publish (`publish.yaml`)

**Triggers:** Push to `main` branch

**Purpose:** Publish wrapper charts and verify patched images in GHCR

**Flow:**
1. Package and push wrapper charts to OCI registry
2. Verify all patched images exist
3. Generate site catalog and deploy to GitHub Pages

---

## Self-Maintenance Flow

### Scenario 1: Renovate Updates Chart Version

```
1. Renovate opens PR: Chart.yaml updated (prometheus 28.9.1 → 29.0.0)
   ↓
2. patch-matrix.yaml triggers
   ↓
3. Discover job scans new chart, finds 15 images → matrix
   ↓
4. 15 parallel patch jobs run (one image per runner)
   ↓
5. Assemble job creates wrapper charts, commits to PR
   ↓
6. Review & merge PR
   ↓
7. publish.yaml publishes to GHCR
```

### Scenario 2: New CVE Discovered

```
1. Scheduled scan runs nightly
   ↓
2. Discover finds all images → matrix
   ↓
3. Patch jobs scan existing patched images for new vulns
   ↓
4. Images with new fixable CVEs get re-patched
   ↓
5. Assemble creates PR if charts changed
   ↓
6. Review & merge PR
   ↓
7. publish.yaml publishes updated charts
```

## Data Flow Between Jobs

```
Discover ──► .verity/manifest.json   (artifact: verity-manifest)
         ──► matrix JSON             (output: matrix)

Patch[i] ──► .verity/results/<image>.json  (artifact: patch-result-<name>)
         ──► .verity/reports/<image>.json

Assemble ◄── manifest.json + all results + all reports
         ──► charts/<name>/Chart.yaml
         ──► charts/<name>/values.yaml
         ──► charts/<name>/reports/*.json
```

## Permissions Required

All workflows use `GITHUB_TOKEN` with these permissions:

- `contents: write` - For committing wrapper charts
- `pull-requests: write` - For scheduled-scan to create PRs
- `packages: write` - For GHCR image and chart publishing

## Configuration

### Registry Settings

Patched images: `ghcr.io/<owner>/<image-name>:<tag>-patched`
Charts: `oci://ghcr.io/<owner>/charts/<chart-name>`

### Matrix Settings

```yaml
strategy:
  matrix: ${{ fromJson(needs.discover.outputs.matrix) }}
  fail-fast: false    # Don't cancel other images if one fails
  max-parallel: 10    # Limit concurrent runners
```

### Scheduled Scan Frequency

Edit `.github/workflows/scheduled-scan.yaml`:
```yaml
on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM UTC
```

## Troubleshooting

### Matrix job fails for one image

Other images continue due to `fail-fast: false`. The assemble job still runs and creates wrapper charts using the successful results. Check the failed job's logs for details.

### Discover step finds 0 images

- Verify `Chart.yaml` has valid dependencies
- Check Helm registry login succeeded
- Run `./verity -scan` locally to debug

### Assemble step has missing results

This happens when patch jobs fail. Assemble handles missing results gracefully — images without results are treated as unpatched.
