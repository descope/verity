<p align="center">
  <img src="site/public/logo.svg" alt="Verity Logo" width="420">
</p>

<h1 align="center">Verity</h1>
<p align="center"><strong>Self-maintaining registry of security-patched container images</strong></p>
<p align="center">
  <a href="#quick-start">Quick Start</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#benefits">Benefits</a> •
  <a href="#documentation">Documentation</a>
</p>

---

Verity automatically scans container images for vulnerabilities, patches them using
[Copa](https://github.com/project-copacetic/copacetic), and publishes patched versions to GitHub Container Registry (GHCR).

## Quick Start

### Use a Patched Image

```bash
# Pull a patched image
docker pull ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Use in Kubernetes
kubectl set image deployment/prometheus \
  prometheus=ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Use in Docker Compose
services:
  prometheus:
    image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
```

### Run Locally

```bash
# List images to patch
./verity list

# Discover images for CI
./verity discover

# Patch a single image
./verity patch \
  --image "quay.io/prometheus/prometheus:v3.9.1" \
  --registry ghcr.io/myorg \
  --buildkit-addr docker-container://buildkitd \
  --result-dir ./results
```

## How It Works

```text
Chart.yaml (chart dependencies)
        ↓
  Scan charts (verity scan)
        ↓
values.yaml (discovered images)
        ↓
  Discover (parse + apply overrides → matrix.json)
        ↓
  Patch (parallel: trivy + copa)
        ↓
  Sign & Attest (cosign + SLSA + SBOM)
        ↓
  Published to ghcr.io/verity-org
```

### Image Sources

**Chart.yaml** - Defines Helm charts to track (Renovate updates these)
**values.yaml** - Auto-generated list of all images from charts (single source of truth for patching)

**Example values.yaml:**

```yaml
# Image tag overrides for Copa compatibility
overrides:
  timberio/vector:
    from: "distroless-libc"  # Copa can't patch distroless
    to: "debian"              # Use debian variant instead

# Images to patch
prometheus:
  image:
    registry: quay.io
    repository: prometheus/prometheus
    tag: "v3.9.1"

grafana:
  image:
    registry: docker.io
    repository: grafana/grafana
    tag: "12.3.3"
```

### Image Naming Convention

- **Source**: `quay.io/prometheus/prometheus:v3.9.1`
- **Patched**: `ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched`

All patched images get a `-patched` suffix.

## Automation

Verity is **fully automated** with GitHub Actions:

### 1️⃣ Daily Vulnerability Scans

- Scans for new vulnerabilities
- Creates PR if patches available
- Runs daily at 2 AM UTC

### 2️⃣ Auto-Scan Charts (Renovate + Workflow)

- Renovate updates Chart.yaml (chart versions)
- Workflow scans charts and updates values.yaml
- Commits to PR
- Ready to merge!

### 3️⃣ Auto-Patch Images (On values.yaml Change)

- values.yaml changes trigger patching workflow
- All images patched in parallel
- Results committed to PR

### 4️⃣ Publish to GHCR (On Merge)

- Patched images pushed to GitHub Container Registry
- Images signed with cosign (keyless)
- SLSA L3 provenance + SBOM + vulnerability reports attached
- Site catalog updated

See [WORKFLOWS.md](WORKFLOWS.md) for details.

## Benefits

### For Image Consumers

✅ Security-patched container images
✅ Automated vulnerability monitoring
✅ Drop-in replacements for upstream images
✅ SLSA L3 build provenance
✅ Signed with cosign (Sigstore)
✅ Full SBOM attestations
✅ Zero-trust supply chain (verify everything yourself)

## Architecture

### Components

- **Verity** (Go) - Image scanner and patcher
- **Trivy** - Vulnerability scanner
- **Copa** - Microsoft's container patching tool
- **BuildKit** - Image building
- **Cosign** - Image signing (Sigstore)

### Workflow System

```text
┌──────────────┐
│  Renovate    │ Updates values.yaml
└──────┬───────┘
       ↓
┌──────────────────────┐
│ scan-and-patch.yaml  │ Auto-patches (matrix)
└──────┬───────────────┘
       ↓
┌────────────────┐
│ Merge to main  │
└──────┬─────────┘
       ↓
┌────────────────┐
│ Push to GHCR   │ Signed + attested
└────────────────┘
```

Plus daily scheduled scans for continuous monitoring.

## Usage

### Add Charts to Monitor

Edit `Chart.yaml` to add dependencies:

```yaml
dependencies:
  - name: my-chart
    version: "1.2.3"
    repository: https://charts.example.com
```

Then run:

```bash
make scan  # Updates values.yaml with discovered images
```

Workflows handle this automatically on merge.

### Updating Image Versions

Renovate handles this automatically. For manual updates:

```bash
# Update values.yaml image tags
# Create PR → scan-and-patch validates → merge → publishes
```

### Configuration

**Registry:**
Set via `-registry` flag (e.g. `ghcr.io/your-org`).

**Scan Schedule:**
Edit `.github/workflows/scan-and-patch.yaml`:

```yaml
schedule:
  - cron: '0 2 * * *'  # Daily at 2 AM UTC
```

## Installation

### Prerequisites

- Go 1.25+
- Docker
- BuildKit (for patching)

### Build

```bash
go build -o verity .
```

### Docker

```bash
docker run --rm -v $(pwd):/workspace \
  ghcr.io/verity-org/verity:latest \
  list --images /workspace/values.yaml
```

## CLI Reference

```text
verity - Self-maintaining registry of security-patched container images

Commands:
  discover    Scan images and output a GitHub Actions matrix
  patch       Patch a single container image
  list        List images from values.yaml (dry run)
  catalog     Generate site catalog JSON from patch reports

Use "verity [command] --help" for command-specific options.
```

**Common Options:**

- `--images, -i` - Path to images values.yaml (default: "values.yaml")
- `--registry` - Target registry for patched images (e.g. ghcr.io/verity-org)

**Examples:**

```bash
# List images
./verity list

# Discover for CI
./verity discover --discover-dir .verity

# Patch single image (in CI matrix)
./verity patch \
  --image "quay.io/prometheus/prometheus:v3.9.1" \
  --registry ghcr.io/myorg \
  --buildkit-addr docker-container://buildkitd \
  --result-dir ./results

# Generate site catalog
./verity catalog \
  --output site/src/data/catalog.json \
  --registry ghcr.io/verity-org \
  --reports-dir .verity/reports
```

## Development

### Run Tests

```bash
go test ./...
```

### Validate Workflows

```bash
# Check YAML syntax
actionlint .github/workflows/*.yaml
```

### Local Testing

Test without touching external registries using Docker Compose:

```bash
# Start local registry + BuildKit
make up

# Run integration tests (fast - no patching)
make test-local

# Test patching with local registry
make test-local-patch

# Stop services
make down
```

**Manual testing:**

```bash
# Start services
docker compose up -d

# Test discover
./verity discover

# Test list
./verity list

# Test patch with local registry
./verity patch \
  --image "docker.io/library/nginx:1.29.5" \
  --registry "localhost:5555/verity" \
  --buildkit-addr "tcp://localhost:1234" \
  --result-dir .verity/results

# Check results
ls -la .verity/results/
curl http://localhost:5555/v2/_catalog

# Cleanup
docker compose down -v
```

## Documentation

- [WORKFLOWS.md](WORKFLOWS.md) - Complete workflow automation guide
- [CONTRIBUTING.md](CONTRIBUTING.md) - Development setup and guidelines
- [ARCHITECTURE.md](ARCHITECTURE.md) - System architecture (images-only)
- [MIGRATION_COMPLETE.md](MIGRATION_COMPLETE.md) - Migration from charts to images

## Security

### Vulnerability Scanning

Every patch run includes:

- Trivy vulnerability reports attached as **in-toto attestations**
- SBOM (CycloneDX) attestations
- SLSA L3 build provenance attestations
- CVE details and CVSS scores
- Fixable vs unfixable vulnerabilities

### Image Trust

Patched images are:

1. Built from official upstream images
2. Scanned with Trivy (open source)
3. Patched with Copa (Microsoft, open source)
4. Pushed to GHCR with `-patched` suffix
5. Signed with cosign (keyless, Sigstore)
6. Attested with build provenance, SBOM, and vulnerability reports
7. Never modify upstream images

### Supply Chain

Verify patches yourself:

```bash
# Verify image signature
cosign verify \
  --certificate-identity-regexp "https://github.com/verity-org/verity/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Verify build provenance
gh attestation verify \
  oci://ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched \
  --owner verity-org

# View vulnerability report attestation
cosign verify-attestation --type vuln \
  --certificate-identity-regexp "https://github.com/verity-org/verity/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Compare to original
docker pull quay.io/prometheus/prometheus:v3.9.1
docker pull ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
```

## FAQ

**Q: What types of vulnerabilities can Copa patch?**
A: Copa patches OS-level packages (apt, yum, apk). It cannot patch application vulnerabilities in compiled binaries.

**Q: Will this patch ALL vulnerabilities?**
A: No. Only vulnerabilities with available package updates. Some images may have unfixable CVEs.

**Q: How do I use patched images in my deployments?**
A: Just change the image reference to use `ghcr.io/verity-org/` instead of the original registry.

**Q: What if I don't want to auto-merge security updates?**
A: Edit `.github/renovate.json` and set `automerge: false`.

**Q: How do I add more images?**
A: Just add them to `values.yaml`. Workflows automatically handle any number of images.

**Q: Can I run this without GitHub Actions?**
A: Yes! Verity is a standalone CLI tool. Run it manually or integrate with any CI system.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Open a pull request

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

[MIT License](LICENSE)

## Acknowledgments

- [Copa](https://github.com/project-copacetic/copacetic) - Microsoft's container patching tool
- [Trivy](https://github.com/aquasecurity/trivy) - Vulnerability scanner
- [Sigstore](https://www.sigstore.dev/) - Keyless signing infrastructure
- [SLSA](https://slsa.dev/) - Supply-chain Levels for Software Artifacts

---

<p align="center">
  <strong>Built to make Kubernetes more secure</strong><br>
  <sub>Powered by Copa, Trivy, and Sigstore</sub>
</p>
