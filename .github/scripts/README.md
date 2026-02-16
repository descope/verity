# GitHub Actions Scripts

This directory contains shell scripts used by GitHub Actions workflows.

## Issue Handling - Automated PR Creation

### Adding a New Chart (`add-chart-dependency.sh`)

**Triggered by:** Creating an issue with the "new-chart" label

**Flow:**
```
User creates issue → Script adds to Chart.yaml → PR created → Merge → Renovate keeps updated
```

**What happens after merge:**
1. Renovate auto-updates chart versions
2. `update-images` workflow scans chart for images
3. Updates `values.yaml` automatically
4. `scan-and-patch` workflow patches and publishes images + charts

### Adding a Standalone Image (`add-standalone-image.sh`)

**Triggered by:** Creating an issue with the "new-image" label

**Flow:**
```
User creates issue → Script adds to charts/standalone/values.yaml → PR created → Merge → Image gets scanned & patched
```

**What happens after merge:**
1. Image is added to the standalone chart (`charts/standalone/`)
2. `update-images` workflow scans all charts including standalone
3. Updates root `values.yaml` with standalone images
4. `scan-and-patch` workflow patches and publishes the image
5. Renovate auto-updates the image tag in `charts/standalone/values.yaml`

**Architecture:** Standalone images are managed through a local Helm chart to ensure they survive `scan` regeneration.

## Workflow Scripts

All scripts follow best practices: `set -euo pipefail`, shellcheck validated, clear error messages.

### Discovery & Matrix
- `discover-images.sh` - Image discovery and matrix generation

### Image Operations
- `get-image-digest.sh` - Extract digest from results  
- `sign-image.sh` - Cosign signature
- `generate-image-sbom.sh` - Trivy SBOM
- `attest-image-vulnerability.sh` - Vulnerability attestation

### Chart Operations
- `assemble-charts.sh` - Assemble and publish charts
- `get-chart-digest.sh` - Get OCI chart digest
- `sign-chart.sh` - Cosign chart signature
- `attest-chart-vulnerability.sh` - Chart vulnerability attestation

### Utilities
- `install-copa.sh` - Install Copa
- `parse-*.sh` - Issue form parsers
- `verify-*.sh` - Verification scripts

## Development

```bash
# Lint all scripts
make lint-scripts

# CI automatically runs shellcheck on all *.sh files
```
