# PR Testing Guide

## Overview

Pull requests automatically test the Copa bulk config pipeline without pushing to the production registry (`ghcr.io/verity-org`).

## How It Works

### Production Mode (main branch)
- Uses `ghcr.io/verity-org` registry
- Patches images with Copa
- Signs with cosign
- Attests SBOMs and vulnerability reports
- Deploys site to GitHub Pages

### PR Testing Mode (pull requests)
- Uses local Docker registry (`localhost:5000/verity-test`)
- Patches images with Copa (validates config)
- Scans with Trivy (validates scanning works)
- Generates catalog (validates data pipeline)
- Builds site (validates frontend integration)
- **Skips:** Signing, attestation, and publishing

## What Gets Tested in PRs

✅ Copa bulk config syntax and processing
✅ Image patching logic
✅ Trivy vulnerability scanning
✅ Catalog JSON generation
✅ Site build process
✅ Overall pipeline orchestration

❌ Image signing (requires production credentials)
❌ Registry publishing (uses ephemeral local registry)
❌ Site deployment (only on main)

## Reviewing PR Results

### GitHub Actions Summary
Each PR run includes a summary showing:
- Number of images processed
- Catalog statistics (total vulnerabilities, fixable count)
- Validation status of each pipeline step

### Downloadable Artifacts
PR runs upload test artifacts (retained for 7 days):
- `images.json` - List of processed images
- Trivy reports (`.verity/reports/*.json`)
- Generated catalog (`catalog.json`)

## Triggering PR Tests

PR tests run automatically when you modify:
- `copa-config.yaml` (image configuration)
- `.github/workflows/scan-and-patch.yaml` (workflow changes)
- `internal/**` (Go code changes)
- `cmd/**` (CLI changes)

## Local Testing

To test the workflow locally before pushing:

```bash
# Start local registry
docker run -d -p 5000:5000 --name registry registry:2

# Configure Docker for insecure local registry
# (Add to /etc/docker/daemon.json)
{
  "insecure-registries": ["localhost:5000"]
}

# Test Copa patching locally
export REGISTRY=localhost:5000/verity-test
copa patch --config copa-config.yaml --push \
  --ignore-errors --addr docker-container://buildx_buildkit_*

# Verify images were pushed
crane ls localhost:5000/verity-test/prometheus
```

## Optimizing PR Test Speed

If PR tests take too long, consider:

1. **Reduce test image count** - Create `copa-config-pr.yaml` with subset of images
2. **Cache Copa binary** - Workflow caches by commit hash
3. **Parallel processing** - Copa bulk mode already maximizes concurrency

## Troubleshooting

### "Registry connection failed"
- Check that the local registry service started successfully
- Verify Docker daemon has insecure-registries configured

### "No images processed"
- Verify `copa-config.yaml` syntax
- Check workflow logs for Copa errors
- Ensure BuildKit is running

### "Catalog generation failed"
- Check that Trivy reports were created
- Verify `images.json` has valid content
- Review verity CLI logs
