# GitHub Actions Scripts

This directory contains shell scripts used by GitHub Actions workflows.

## Issue Handling - Automated PR Creation

### Adding a Standalone Image (`add-standalone-image.sh`)

**Triggered by:** Creating an issue with the "new-image" label

**Flow:**

```
User creates issue → Script adds to copa-config.yaml → PR created → Merge → Image gets scanned & patched
```

**What happens after merge:**

1. Image is added to `copa-config.yaml` under `images:`
2. **scan-and-patch workflow** patches and publishes the image to GHCR

## Workflow Scripts

All scripts follow best practices: `set -euo pipefail`, shellcheck validated, clear error messages.

### Discovery & Matrix

- `copa-discover.sh` - Copa dry-run, matrix generation, skip detection

### Image Operations

- `get-image-digest.sh` - Extract digest from results
- `sign-image.sh` - Cosign signature
- `generate-image-sbom.sh` - Trivy SBOM
- `attest-image-vulnerability.sh` - Vulnerability attestation

### Utilities

- `parse-*.sh` - Issue form parsers
- `verify-*.sh` - Verification scripts

## Development

```bash
# Lint all scripts
make lint-shell

# CI automatically runs shellcheck on all *.sh files
```
