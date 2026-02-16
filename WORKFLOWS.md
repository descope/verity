# Workflow Architecture

Verity uses a streamlined workflow system to automatically scan charts, discover images, and patch them in
parallel using GitHub Actions.

## Overview

```text
┌──────────────────────────────────────────────────────────────┐
│                      Renovate Bot                            │
│  Updates Chart.yaml dependencies (chart versions)            │
└────────────────────────┬─────────────────────────────────────┘
                         │
                         ▼
         ┌───────────────────────────────┐
         │  update-images.yaml           │
         │  • helm dependency update     │
         │  • verity scan (extract imgs) │
         │  • commit values.yaml         │
         └───────────┬───────────────────┘
                     │
                     ▼
         ┌───────────────────────────────┐
         │  scan-and-patch.yaml          │
         │  Job 1: Discover              │
         │    • Parse values.yaml        │
         │    • Apply overrides          │
         │    • Generate matrix          │
         │                               │
         │  Job 2: Patch (matrix)        │
         │    ┌────────┐  ┌────────┐    │
         │    │Image A │  │Image B │... │
         │    │trivy   │  │trivy   │    │
         │    │copa    │  │copa    │    │
         │    │sign    │  │sign    │    │
         │    └────────┘  └────────┘    │
         │                               │
         │  Job 3: Generate Catalog      │
         │    • Collect reports          │
         │    • Update site data         │
         │    • Deploy to GitHub Pages   │
         └───────────────────────────────┘
                     │
                     ▼
         ┌───────────────────────────────┐
         │  ghcr.io/verity-org          │
         │  • Patched images             │
         │  • Signed with cosign         │
         │  • SLSA attestations          │
         │  • SBOM + vuln reports        │
         └───────────────────────────────┘
```

## Core Commands

```bash
# Scan charts for images
./verity scan --chart . --output values.yaml

# Discover images and generate matrix
./verity discover --images values.yaml --discover-dir .verity

# Patch a single image
./verity patch \
  --image "quay.io/prometheus/prometheus:v2.45.0" \
  --registry ghcr.io/verity-org \
  --buildkit-addr docker-container://buildkitd \
  --result-dir .verity/results \
  --report-dir .verity/reports

# List images (dry run)
./verity list --images values.yaml

# Generate site catalog
./verity catalog \
  --output site/src/data/catalog.json \
  --images values.yaml \
  --registry ghcr.io/verity-org \
  --reports-dir .verity/reports
```

## Workflows

### 1. Update Images (`update-images.yaml`)

**Triggers:**

- Pull requests modifying `Chart.yaml` or `Chart.lock`
- Push to main (Chart.yaml/Chart.lock changes)
- Manual (`workflow_dispatch`)

**Purpose:** Auto-update values.yaml when chart dependencies change

**Flow:**

1. Renovate opens PR updating Chart.yaml
2. Workflow downloads chart dependencies
3. Scans all charts for images
4. Updates values.yaml with discovered images
5. Commits to PR

**Result:** values.yaml is always in sync with Chart.yaml

---

### 2. Scan and Patch (`scan-and-patch.yaml`)

**Triggers:**

- Pull requests modifying `values.yaml`
- Push to main (values.yaml changes)
- Daily schedule (2 AM UTC)
- Manual (`workflow_dispatch`)

**Purpose:** Patch all images when values.yaml changes or on schedule

**Flow:**

#### Job 1: Discover

```bash
./verity discover --images values.yaml --discover-dir .verity
```

- Parses values.yaml
- Applies overrides (e.g., distroless → debian)
- Generates matrix.json for parallel patching
- Outputs manifest.json with all image metadata

#### Job 2: Patch (Matrix)

```bash
./verity patch --image ${{ matrix.image_ref }} ...
```

- **Matrix strategy** - Each image gets its own runner
- Runs in parallel (fail-fast: false)
- Per image:
  1. Pull source image
  2. Scan with Trivy
  3. Patch with Copa
  4. Push to ghcr.io/verity-org
  5. Sign with cosign
  6. Attest (provenance, SBOM, vuln report)

**Why matrix?** Resource isolation + parallelization. Each image gets 2 vCPU / 7GB RAM.

#### Job 3: Generate Catalog

```bash
./verity catalog --output site/src/data/catalog.json ...
```

- Collects all patch results
- Aggregates vulnerability data
- Updates site catalog
- Deploys to GitHub Pages

**Result:** Patched images published to GHCR with full attestations

---

## Data Flow

```text
Chart.yaml
    ↓ (helm dependency update)
charts/*.tgz
    ↓ (verity scan)
values.yaml ──────────────┐
    ↓ (verity discover)   │
manifest.json             │
matrix.json               │
    ↓ (parallel patching) │
.verity/results/*.json    │
.verity/reports/*.json    │
    ↓ (verity catalog) ───┤
site/src/data/catalog.json
```

**Key files:**

- **Chart.yaml** - Source of truth for chart dependencies
- **values.yaml** - Centralized list of all images to patch
- **manifest.json** - Full image metadata with paths
- **matrix.json** - GitHub Actions matrix format
- **results/*.json** - Patch results (registry, tag, digest)
- **reports/*.json** - Trivy vulnerability reports

---

## Image Overrides

Copa cannot patch distroless/Alpine/scratch images. Use overrides in values.yaml:

```yaml
# values.yaml
overrides:
  timberio/vector:
    from: "distroless-libc"  # Unpatchable
    to: "debian"              # Patchable variant

timberio-vector:
  image:
    repository: timberio/vector
    tag: "0.50.0-distroless-libc"
```

During discovery, the tag transforms to `0.50.0-debian` so Copa patches the right variant.

---

## Self-Maintenance Flow

### Scenario 1: Renovate Updates Chart

```text
1. Renovate: Chart.yaml updated (prometheus 28.9.1 → 29.0.0)
   ↓
2. update-images.yaml: Scans new chart → updates values.yaml
   ↓
3. scan-and-patch.yaml: Triggers on values.yaml change
   ↓
4. Discover: Parses images, applies overrides → matrix
   ↓
5. Patch (matrix): 20 images patched in parallel
   ↓
6. Catalog: Site updated with new patch data
   ↓
7. Merge PR → Images live on GHCR
```

### Scenario 2: Daily Vulnerability Scan

```text
1. Cron: 2 AM UTC
   ↓
2. scan-and-patch.yaml: Scans all images for new CVEs
   ↓
3. Images with fixable vulns get re-patched
   ↓
4. Catalog updated if vulnerabilities found/fixed
```

---

## Configuration

### Registry

```yaml
env:
  REGISTRY: ghcr.io/verity-org
```

Patched images: `ghcr.io/verity-org/<repo>/<image>:<tag>-patched`

### Matrix Settings

```yaml
strategy:
  matrix: ${{ fromJson(needs.discover.outputs.matrix) }}
  fail-fast: false    # Continue patching other images if one fails
```

### Schedule

```yaml
on:
  schedule:
    - cron: '0 2 * * *'  # Daily at 2 AM UTC
```

---

## Permissions

```yaml
permissions:
  contents: write       # Commit to PR
  packages: write       # Push to GHCR
  pull-requests: write  # Create/update PRs
  id-token: write       # OIDC signing
  attestations: write   # GitHub Attestations API
```

---

## Local Testing

Use `act` to test workflows locally:

```bash
# Install act
brew install act

# Test chart scanning
make test-update-images

# Test image patching (requires local registry)
make up  # Start local registry + BuildKit
make test-scan-and-patch
```

See [LOCAL_TESTING.md](LOCAL_TESTING.md) for details.

---

## Troubleshooting

### Matrix job fails for one image

Other images continue (`fail-fast: false`). Check failed job logs. Common issues:

- Image doesn't exist / no access
- Copa can't patch distroless images (use overrides)
- BuildKit connection issues

### Discover finds 0 images

- Verify Chart.yaml has dependencies
- Run `helm dependency update` manually
- Check `verity scan --chart .` locally

### Override not applied

- Verify override syntax in values.yaml
- Check discover job logs for "Loaded N override(s)"
- Ensure repository name matches exactly
