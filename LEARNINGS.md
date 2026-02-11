# Copa (Copacetic) - Container Image Vulnerability Patching

## What is Copa?

Copa is an open-source CLI tool written in Go, developed by Microsoft, and now a **CNCF sandbox project**. It directly patches OS-level vulnerabilities in container images **without requiring a full rebuild**. It operates on top of [BuildKit](https://github.com/moby/buildkit) (Docker's default builder) and uses vulnerability reports from scanners like [Trivy](https://github.com/aquasecurity/trivy) to determine what needs patching.

- **GitHub**: https://github.com/project-copacetic/copacetic
- **Docs**: https://project-copacetic.github.io/copacetic/website/

## Why Copa Exists

The gap between vulnerability disclosure and active exploitation is shrinking. Traditional remediation requires waiting for upstream base image updates to propagate through the supply chain, or running full image rebuilds. Copa addresses this by:

- **Patching in-place** - no full rebuild pipeline needed
- **Creating minimal patch layers** - only changed files are added, preserving layer caching
- **Enabling DevSecOps independence** - engineers can patch images they don't publish
- **Reducing turnaround time** - faster than full rebuilds

## How It Works (Architecture)

Copa operates as a three-stage engine:

1. **Parse vulnerability report** - Extracts required package updates from scanner output (e.g., Trivy JSON). Supports custom scanner adapters via templates.
2. **Acquire packages** - Uses the appropriate OS-level package manager (`apt`, `apk`, `yum`, etc.) to fetch updates.
3. **Apply patches** - Uses BuildKit's diff and merge capabilities to create a new image layer containing only the patched files, which is appended to the original image.

## Supported Platforms

- **Package managers**: apt (Debian/Ubuntu), apk (Alpine), yum/rpm (RHEL, CentOS, Fedora)
- **Distroless images**: Supports DPKG and RPM-based distroless images by spinning up a build tooling container
- **Scanners**: Trivy (primary), extensible via custom adapters

## Scope and Limitations

- **Patches OS-level packages only** (e.g., openssl, curl, libc)
- **Does NOT patch application-level vulnerabilities** (e.g., Python pip packages, Go modules, npm packages)
- Only vulnerabilities with available fixes can be patched (use `--ignore-unfixed` in Trivy to filter)

## Installation

```bash
# From GitHub releases (recommended)
# Download from https://github.com/project-copacetic/copacetic/releases

# Build from source
git clone https://github.com/project-copacetic/copacetic
cd copacetic && make
sudo cp dist/linux_amd64/release/copa /usr/local/bin/

# Homebrew (macOS/Linux)
brew install copa

# Docker Desktop Extension (no CLI needed)
# Install from Docker Marketplace
```

## CLI Usage

### Prerequisites

A running BuildKit instance is required. Start one with:

```bash
# Docker container method
docker run --detach --privileged --name buildkit \
  --entrypoint buildkitd --rm "moby/buildkit:v0.12.3"

# Or with TCP port
docker run --detach --rm --privileged \
  -p 127.0.0.1:8888:8888/tcp \
  --name buildkitd --entrypoint buildkitd \
  "moby/buildkit:v0.12.3" --addr tcp://0.0.0.0:8888

# Or via Docker Buildx
docker buildx create --use --name builder
```

### Step 1: Scan with Trivy

```bash
trivy image --vuln-type os --ignore-unfixed -f json -o report.json nginx:1.21.6
```

### Step 2: Patch with Copa

```bash
copa patch \
  --image docker.io/library/nginx:1.21.6 \
  --report report.json \
  --tag 1.21.6-patched \
  --addr docker-container://buildkit \
  --timeout 30m
```

### Step 3: Verify

```bash
trivy image --vuln-type os --ignore-unfixed nginx:1.21.6-patched
```

### `copa patch` Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--image` | `-i` | Container image reference to patch |
| `--report` | `-r` | Vulnerability report JSON from scanner |
| `--tag` | `-t` | Tag for the patched output image |
| `--addr` | `-a` | BuildKit instance address |
| `--timeout` | | Patch timeout (default: 5m) |
| `--debug` | | Enable debug output |

### BuildKit Address Formats

| Format | Description |
|--------|-------------|
| `docker-container://name` | BuildKit in a Docker container |
| `buildx://builder-name` | Buildx builder instance |
| `tcp://host:port` | BuildKit over TCP |
| `unix:///path/to/socket` | BuildKit over Unix socket |
| `nerdctl-container://name` | BuildKit via nerdctl |
| `kubepod://pod-name` | BuildKit in a Kubernetes pod |

If no `--addr` is specified, Copa auto-connects in order: Docker BuildKit endpoint (Docker v24.0+ with containerd snapshotter), selected buildx builder, BuildKit daemon at `/run/buildkit/build`.

## How This Repo Uses Copa

This repository (`verity`) uses Copa indirectly through **[Helmper](https://github.com/ChristofferNissen/helmper)** - a tool that imports Helm charts into OCI registries with optional vulnerability patching.

### The Pipeline

1. **Helmper** reads `helmper.yaml` to discover which Helm charts to process (currently: Prometheus v25.8.0)
2. **Trivy** server (running as a service container on port 8887) scans all container images referenced by the charts
3. **Copacetic** patches any OS-level vulnerabilities using BuildKit
4. **Cosign** signs the patched images with keyless signing
5. **Oras** pushes the patched, signed images to `ghcr.io/descope`

### Configuration (`helmper.yaml`)

```yaml
import:
  copacetic:
    enabled: true
    ignoreErrors: false
    buildkitd:
      addr: docker-container://buildx_buildkit_default
    trivy:
      addr: http://localhost:8887
      ignoreUnfixed: true    # Only patch fixable vulns
    output:
      tars:
        folder: ./.helmper-out/tars
      reports:
        folder: ./.helmper-out/reports
```

### CI/CD Workflow (`.github/workflows/helmper.yaml`)

- **Triggers**: PR changes, daily at 2 AM, manual dispatch
- **Services**: Trivy v0.50.4 server container
- **Tools installed**: Docker Buildx, Helmper (latest), Cosign
- **Artifacts**: Vulnerability reports retained for 30 days

## Integration Patterns

### GitHub Actions (Copa directly)

```yaml
- uses: project-copacetic/copa-action@v1
  with:
    image: myimage:latest
    report: trivy-report.json
    tag: patched
```

### Helmper (wraps Copa + Trivy + Cosign)

Helmper orchestrates the full scan-patch-sign-push workflow for all images in Helm charts. It connects to Trivy and BuildKit via gRPC and can run without root privileges.

## Key Takeaways

1. Copa fills a critical gap: patching container vulnerabilities **without rebuilding** images
2. It only handles **OS-level** packages - app-level vulns need other tools
3. BuildKit is a hard requirement - Copa cannot function without it
4. The Trivy + Copa + Cosign toolchain provides scan + patch + sign in a single pipeline
5. Helmper wraps this toolchain for Helm chart-specific workflows, which is how this repo uses it
