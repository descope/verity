# GitHub Actions Scripts

Shell scripts used by GitHub Actions workflows. All follow `set -euo pipefail` and are shellcheck validated.

## Workflow Scripts

| Script | Used by | Purpose |
|--------|---------|---------|
| `copa-discover.sh` | `patch-matrix.yaml` / scan | Copa dry-run, matrix JSON generation |
| `scan-chart-images.sh` | `patch-matrix.yaml` / scan | Trivy-scan chart images missing reports |
| `patch-image.sh` | `patch-matrix.yaml` / patch | Copa patch + crane fallback for one platform image |
| `create-manifests.sh` | `patch-matrix.yaml` / combine | Multi-platform manifest list creation |
| `sign-manifests.sh` | `patch-matrix.yaml` / combine | Cosign keyless signing of all manifests |
| `build-attestation-matrix.sh` | `patch-matrix.yaml` / combine | Build attest job matrix from manifest digests |
| `generate-manifest-report.sh` | `patch-matrix.yaml` / combine | Per-image manifest report JSON files |
| `generate-catalog.sh` | `patch-matrix.yaml` / assemble | Build catalog JSON from scan + patch results |
| `pr-summary.sh` | `patch-matrix.yaml` / assemble | Write pipeline summary to step summary |
| `parse-image-issue-form.sh` | `new-issue.yaml` | Parse image fields from GitHub issue form body |
| `add-standalone-image.sh` | `new-issue.yaml` | Add image to `copa-config.yaml` and open PR |

## Utility Scripts

| Script | Purpose |
|--------|---------|
| `verify-artifacts.sh` | Verify cosign signatures and GitHub attestations for published images |

## Development

```bash
# Lint all scripts
make lint-shell
```
