<p align="center">
  <img src="site/public/logo.svg" alt="Verity Logo" width="420">
</p>

<h1 align="center">Verity</h1>
<p align="center"><strong>Self-maintaining registry of security-patched container images</strong></p>
<p align="center">
  <a href="#the-problem">The Problem</a> •
  <a href="#quick-start">Quick Start</a> •
  <a href="#how-it-works">How It Works</a> •
  <a href="#verify-the-supply-chain">Verify</a> •
  <a href="ARCHITECTURE.md">Architecture</a>
</p>

---

## The Problem

Container images ship with packages that accumulate CVEs daily — both OS-level
(apt, yum, apk) and application-level (pip, etc.).
Upstream maintainers patch on their own schedule — if at all.
Organizations are left choosing between manually rebuilding every image
they depend on or running known-vulnerable containers in production.

Verity eliminates that trade-off. It continuously scans container images for
vulnerabilities, patches them in-place using [Copa](https://github.com/project-copacetic/copacetic)
(no Dockerfile rebuild required), and publishes signed, attested, drop-in
replacements to GitHub Container Registry.

**Browse the catalog:** [verity-org.github.io/verity](https://verity-org.github.io/verity/)

## Quick Start

Replace your image reference. That's it.

```bash
# Pull a patched image
docker pull ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
```

```yaml
# Use in Kubernetes
image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Use in Docker Compose
services:
  prometheus:
    image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
```

All patched images follow the same convention:

| Original | Patched |
| --- | --- |
| `quay.io/prometheus/prometheus:v3.9.1` | `ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched` |
| `docker.io/library/nginx:1.29.5` | `ghcr.io/verity-org/library/nginx:1.29.5-patched` |

## How It Works

```text
  copa-config.yaml
        │
        ▼
   ┌─────────┐     Define Helm charts, standalone images, and tag strategies.
   │ Discover │     Copa auto-discovers all images from chart templates.
   └────┬────┘
        ▼
   ┌─────────┐     Trivy scans every image for known CVEs.
   │  Scan   │     Only images with fixable vulnerabilities proceed.
   └────┬────┘
        ▼
   ┌─────────┐     Copa patches packages in-place —
   │  Patch  │     no Dockerfile rebuild needed.
   └────┬────┘     Parallel matrix jobs across amd64 and arm64.
        ▼
   ┌─────────┐     cosign signs with keyless OIDC (Sigstore).
   │  Sign   │     SLSA L3 provenance, CycloneDX SBOM, and vulnerability
   └────┬────┘     reports attached as in-toto attestations.
        ▼
   ┌─────────┐
   │ Publish │     Pushed to ghcr.io/verity-org with -patched suffix.
   └─────────┘
```

This pipeline runs daily at 02:00 UTC and on every `copa-config.yaml` change.
See [ARCHITECTURE.md](ARCHITECTURE.md) for the full technical breakdown.

### What Can (and Can't) Be Patched

Copa patches **OS-level packages** (`apt`, `yum`/`dnf`, `apk`) and
**Python packages** installed via `pip` (experimental). This covers the majority
of container CVEs.

It **cannot** patch:

- Compiled binaries with statically-linked vulnerable libraries (e.g. Go modules)
- Vulnerabilities without an available upstream package fix
- Distroless images (Verity uses base-image overrides for these)

## Verify the Supply Chain

Every patched image is signed and attested. Verify it yourself:

```bash
# Verify signature (cosign)
cosign verify \
  --certificate-identity-regexp "https://github.com/verity-org/verity/.github/workflows/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Verify build provenance (GitHub CLI)
gh attestation verify \
  oci://ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched \
  --owner verity-org
```

Full compliance details (SLSA, FedRAMP, SOC 2, ISO 27001, NIST, OWASP):
[verity-org.github.io/verity/compliance](https://verity-org.github.io/verity/compliance/)

## Adding Images

### Via GitHub Issue

Open an issue with the **Request New Image** template. Verity creates a PR automatically.

### Via `copa-config.yaml`

```yaml
# Helm chart — Copa auto-discovers all container images from templates
charts:
  - name: prometheus
    version: "28.9.1"
    repository: "oci://ghcr.io/prometheus-community/charts"

# Standalone image with tag strategy
images:
  - name: "nginx"
    image: "mirror.gcr.io/library/nginx"
    platforms: ["linux/amd64", "linux/arm64"]
    tags:
      strategy: "pattern"
      pattern: '^\d+\.\d+\.\d+$'
      maxTags: 3

# Base-image override for images Copa can't patch directly
overrides:
  "timberio/vector":
    from: "distroless-libc"
    to: "debian"
```

Merge the PR and the pipeline handles the rest.

## Running Verity Locally

```bash
# Build
go build -o verity .

# Scan images and generate Trivy reports
./verity scan --config copa-config.yaml --output reports/

# Generate site catalog from patch results
./verity catalog \
  --images-json images.json \
  --output site/src/data/catalog.json \
  --registry ghcr.io/verity-org \
  --reports-dir reports/
```

For local patching with a Docker registry and BuildKit, see the
[development guide](CONTRIBUTING.md#local-testing).

## Documentation

| Document | Description |
| --- | --- |
| [ARCHITECTURE.md](ARCHITECTURE.md) | System design, components, pipeline, CLI reference |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development setup, code quality, PR guidelines |
| [Compliance](https://verity-org.github.io/verity/compliance/) | SLSA, FedRAMP, SOC 2, ISO 27001, OWASP mappings |
| [.github/RENOVATE.md](.github/RENOVATE.md) | Automated dependency update configuration |
| [.github/PR-TESTING.md](.github/PR-TESTING.md) | How PR validation works |

## License

[MIT License](LICENSE)

## Acknowledgments

- [Copa](https://github.com/project-copacetic/copacetic) — Microsoft's container patching tool
- [Trivy](https://github.com/aquasecurity/trivy) — Vulnerability scanner
- [Sigstore](https://www.sigstore.dev/) — Keyless signing infrastructure
- [SLSA](https://slsa.dev/) — Supply-chain Levels for Software Artifacts
