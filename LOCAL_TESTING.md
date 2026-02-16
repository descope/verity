# Local Testing Guide

Test the complete GitHub Actions workflows locally using [act](https://github.com/nektos/act).

## Prerequisites

```bash
# Install act (macOS)
brew install act

# Or see: https://github.com/nektos/act#installation
```

## Quick Start

```bash
# Test the update-images workflow
act pull_request -W .github/workflows/update-images.yaml

# Test the scan-and-patch workflow
act push -W .github/workflows/scan-and-patch.yaml
```

## Testing Workflows

### 1. Test Chart Scanning (update-images workflow)

```bash
# Simulates: Renovate updates Chart.yaml
act pull_request \
  -W .github/workflows/update-images.yaml \
  --container-architecture linux/amd64
```

This runs the workflow that:

1. Downloads chart dependencies
2. Scans for images
3. Updates values.yaml

### 2. Test Image Patching (scan-and-patch workflow)

```bash
# Simulates: values.yaml change triggers patching
act push \
  -W .github/workflows/scan-and-patch.yaml \
  --container-architecture linux/amd64
```

This runs the complete workflow:

1. Discovers images
2. Patches images (in matrix)
3. Signs and attests

**Note:** The full workflow requires:

- Docker access for BuildKit
- Registry access for pushing

### 3. Test Specific Job

```bash
# Test just the discover job
act push \
  -W .github/workflows/scan-and-patch.yaml \
  -j discover

# Test just one patch job
act push \
  -W .github/workflows/scan-and-patch.yaml \
  -j patch
```

## Configuration

Create `.actrc` in the repo root for common settings:

```bash
# .actrc
--container-architecture linux/amd64
--action-offline-mode
--use-gitignore=false
```

## Secrets

For workflows requiring secrets:

```bash
# Create .secrets file
echo "GITHUB_TOKEN=ghp_your_token_here" > .secrets

# Run with secrets
act push -W .github/workflows/scan-and-patch.yaml --secret-file .secrets
```

## Quick Testing Commands

```bash
# Test chart scanning workflow
make test-update-images

# Test patching workflow (requires local registry)
make test-scan-and-patch

# Start local registry for testing
make up
```

## Limitations

`act` runs workflows locally but some features may differ:

- GitHub OIDC signing (cosign) won't work locally
- Attestations API requires GitHub
- Some actions may not work in local containers

For full integration testing, use:

```bash
# Start local infrastructure
make up

# Manual workflow test
./verity scan --chart . -o values.yaml
./verity discover --images values.yaml --discover-dir .verity
./verity patch --image "docker.io/nginx:1.29.5" --registry localhost:5555/verity --buildkit-addr tcp://localhost:1234
```

## Troubleshooting

### act not found

```bash
brew install act
```

### Docker issues

```bash
# act requires Docker Desktop or Docker Engine
docker ps

# Ensure Docker socket is accessible
ls -la /var/run/docker.sock
```

### Workflow fails on secrets

```bash
# Use dummy secrets for local testing
act -W .github/workflows/scan-and-patch.yaml \
  -s GITHUB_TOKEN=dummy
```

## CI vs Local with act

| Aspect | GitHub Actions | Local with act |
| ------ | -------------- | -------------- |
| Workflow execution | ✅ Exact same | ✅ Exact same |
| Secrets | GitHub Secrets | Local .secrets file |
| OIDC signing | ✅ Works | ❌ Not supported |
| Attestations | ✅ Works | ❌ Not supported |
| Matrix parallelization | ✅ Parallel | ⚠️ Sequential by default |

## Next Steps

1. Test workflows locally with `act`
2. Push to GitHub for full CI testing
3. Renovate updates Chart.yaml automatically
4. Workflows run and patch images to GHCR
