# PR Testing Guide

## Overview

Pull requests automatically validate the Verity pipeline without pushing to the production registry (`ghcr.io/verity-org`).

## How It Works

### Production Mode (main branch + nightly)

- `orchestrator.yaml` runs at 02:00 UTC, dispatches `patch-image.yaml` per image
- Pushes to `ghcr.io/verity-org` with `-patched` suffix
- Signs with cosign, attests SBOMs and vulnerability reports
- Catalog assembly and site deploy happen in `build-site.yaml` at 05:00 UTC (decoupled)

### PR Testing Mode (`pr-test.yaml`)

Lightweight validation that runs on pull requests:

- Runs `verity discover` to validate config syntax
- Runs Integer smoke tests via `melange-check.sh` + `melange-build.sh`
- May call `patch-image.yaml` via `workflow_call` with `is-pr: true` to validate patching without publishing
- **Skips:** Signing, attestation, registry publishing, and reports-branch push

## What Gets Tested in PRs

✅ Config validation via `verity discover` (merges `copa-config.yaml` + `Chart.yaml` + `verity.yaml`)
✅ Per-image patching via Copa (validates config + patching logic)
✅ Trivy pre/post scanning
✅ Integer/Wolfi build smoke tests
✅ Per-image multi-arch manifest creation

❌ Image signing (production credentials only)
❌ Registry publishing (uses cache / ephemeral registry only)
❌ Reports-branch push (main only)
❌ Catalog JSON generation and site deployment (`build-site.yaml`, main only)

## Reviewing PR Results

### GitHub Actions Summary

Each PR run includes a summary showing:

- Number of images discovered by `verity discover` (merged from `copa-config.yaml`, `Chart.yaml`, and `verity.yaml`)
- Integer image config validation status
- Smoke test results for sample images
- Copa patching validation (when config changes)

### Downloadable Artifacts

PR runs upload test artifacts (retained for 7 days):

- `images.json` - Merged discovery output from `verity discover` (standalone
  images from `copa-config.yaml` + chart-discovered images from `Chart.yaml` +
  overrides from `verity.yaml`)
- Trivy reports (`trivy-*.json`)
- Copa scan reports (`before.json`, `after.json`) for changed images

## Triggering PR Tests

PR tests run automatically when you modify:

- `copa-config.yaml`
- `images/**`
- `integer.yaml`
- `.github/workflows/pr-test.yaml`
- `.github/workflows/patch-image.yaml`
- `.github/workflows/integer-*.yaml`
- `internal/integer/**`
- `cmd/integer*.go`
- `cmd/discover*.go`
- `cmd/scan*.go`
- `.github/scripts/patch-image.sh`
- `.github/actions/setup-binaries/**`

## Local Testing

For full local development setup with registry and BuildKit:

```bash
# Start local services (registry on :5555, BuildKit on :1234)
make up

# Run your tests...

# Stop local services
make down
```

See [CONTRIBUTING.md](../CONTRIBUTING.md) for detailed setup instructions.

## Optimizing PR Test Speed

If PR tests take too long, consider:

1. **Cache Copa binary** - Workflow caches by commit hash
2. **Parallel processing** - Copa bulk mode already maximizes concurrency

## Troubleshooting

### "Registry connection failed"

- Check that the local registry service started successfully
- Verify Docker daemon has insecure-registries configured for `localhost:5555`

### "No images processed"

- Verify `copa-config.yaml` syntax
- Check workflow logs for Copa errors
- Ensure BuildKit is running

### "Catalog generation failed"

- Check that Trivy reports were created
- Verify `images.json` has valid content
- Review verity CLI logs
