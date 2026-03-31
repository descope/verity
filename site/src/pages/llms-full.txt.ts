import type { APIRoute } from "astro";
import {
  fullCatalog,
  upstreamPath,
  REGISTRY,
  totalImages,
  totalCategories,
  copaCount,
  integerCount,
} from "../data/full-catalog";
import type { FullCatalogImage } from "../data/full-catalog";
import { getChartsCatalog } from "../lib/charts";

/** Format a single catalog image as a markdown list item. */
function formatImage(img: FullCatalogImage): string {
  const label = img.label ?? img.name;
  const source = img.source === "integer" ? "Wolfi-based" : "Copa-patched";
  const parts = [`- **${label}**`];
  parts.push(`— ${source}`);
  if (img.upstream) {
    parts.push(`from \`${img.upstream}\``);
  }
  if (img.variants && img.variants.length > 0) {
    parts.push(`Variants: ${img.variants.join(", ")}.`);
  }

  // Build pull reference
  if (img.source === "integer") {
    const pullName = img.integerName ?? img.name;
    parts.push(`Pull: \`${REGISTRY}/${pullName}\``);
  } else {
    const path = upstreamPath(img);
    parts.push(`Pull: \`${REGISTRY}/${path}\``);
  }

  return parts.join(" ");
}

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const siteUrl = `${origin}${base}`.replace(/\/$/, "");

  // Generate image catalog sections
  const catalogSections = fullCatalog
    .map((category) => {
      const header = `### ${category.label}\n`;
      const images = category.images.map(formatImage).join("\n");
      return header + "\n" + images;
    })
    .join("\n\n");

  // Generate charts section
  const chartsCatalog = getChartsCatalog();
  const chartsSection = chartsCatalog.charts.length > 0 ? (() => {
    const chartEntries = chartsCatalog.charts
      .map((chart) => {
        const installCmd = `helm install ${chart.wrapperName} ${chart.registry}/${chart.wrapperName} --version ${chart.wrapperVersion}`;
        const overrideCount = chart.imageMappings.length + chart.valueOverrides.length;
        let entry = `### ${chart.name} (v${chart.version})\n\n`;
        entry += `- **Wrapper chart**: \`${chart.wrapperName}\` v${chart.wrapperVersion}\n`;
        entry += `- **Install**: \`${installCmd}\`\n`;
        entry += `- **Image overrides**: ${overrideCount}\n`;
        if (chart.repository) {
          entry += `- **Source**: \`${chart.repository}\`\n`;
        }
        if (chart.imageMappings.length > 0) {
          entry += "\n**Image mappings:**\n\n";
          entry += "| Original | Patched |\n|----------|--------|\n";
          entry += chart.imageMappings
            .map((m) => `| \`${m.originalRepo}:${m.originalTag}\` | \`${m.patchedRepo}:${m.patchedTag}\` |`)
            .join("\n");
          entry += "\n";
        }
        return entry;
      })
      .join("\n");
    return chartEntries;
  })() :
    "No wrapper charts have been generated yet. Charts are generated daily at 04:00 UTC after the patching pipeline completes.\n";

  const content = `# Verity — Complete LLM Reference

> Verity is a self-maintaining registry of security-patched container images. It continuously scans container images for CVEs, patches them in-place using Copa (no Dockerfile rebuild required), signs with cosign/Sigstore keyless OIDC, attests with SLSA Level 3 build provenance and CycloneDX SBOMs, and publishes signed drop-in replacements to GitHub Container Registry at ghcr.io/verity-org.

**Registry**: \`ghcr.io/verity-org\`
**Website**: ${siteUrl}
**Repository**: https://github.com/verity-org/verity
**Pipeline schedule**: Daily at 02:00 UTC + on every config change
**Total images**: ${totalImages} across ${totalCategories} categories (${copaCount} Copa-patched, ${integerCount} Wolfi-based)
**Platforms**: linux/amd64, linux/arm64
**Signing**: cosign keyless (Sigstore OIDC), SLSA L3, CycloneDX SBOM, Rekor transparency log

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [How It Works](#how-it-works)
4. [Image Catalog](#image-catalog) (${totalImages} images across ${totalCategories} categories)
5. [Helm Charts](#helm-charts)
6. [Supply Chain Compliance](#supply-chain-compliance)
7. [Architecture](#architecture)
8. [CLI Reference](#cli-reference)
9. [Configuration Format](#configuration-format)
10. [Verification Commands](#verification-commands)
11. [Contributing](#contributing)

---

## Overview

Container images ship with packages — both OS-level (apt, yum, apk) and application-level (pip, etc.) — that accumulate CVEs daily. Upstream maintainers patch on their own schedule, if at all. Organizations are left choosing between manually rebuilding every image they depend on or running known-vulnerable containers in production.

Verity eliminates that trade-off. It continuously scans container images for vulnerabilities, patches them in-place using [Copa](https://github.com/project-copacetic/copacetic) (no Dockerfile rebuild required), and publishes signed, attested, drop-in replacements to GitHub Container Registry.

### Two Types of Images

1. **Copa-patched** (${copaCount} images): Takes the original upstream image and patches OS-level packages in-place. Same layers, same behavior, fewer CVEs. Uses [Project Copacetic](https://github.com/project-copacetic/copacetic).

2. **Wolfi-based** (${integerCount} images): Built from scratch using [Wolfi](https://github.com/wolfi-dev) packages. Contains only the minimum packages needed to run — no shell, no package manager, minimal attack surface by design.

### What Can (and Can't) Be Patched

Copa patches **OS-level packages** (\`apt\`, \`yum\`/\`dnf\`, \`apk\`) and **Python packages** installed via \`pip\` (experimental). This covers the majority of container CVEs.

It **cannot** patch:
- Compiled binaries with statically-linked vulnerable libraries (e.g., Go modules)
- Vulnerabilities without an available upstream package fix
- Distroless images (Verity uses base-image overrides for these)

---

## Quick Start

Replace your image reference. That's it.

\`\`\`bash
# Pull a patched image
docker pull ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
\`\`\`

\`\`\`yaml
# Use in Kubernetes
image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Use in Docker Compose
services:
  prometheus:
    image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
\`\`\`

### Naming Convention

All patched images follow the same convention:

| Original | Patched |
|----------|---------|
| \`quay.io/prometheus/prometheus:v3.9.1\` | \`ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched\` |
| \`docker.io/library/nginx:1.29.5\` | \`ghcr.io/verity-org/library/nginx:1.29.5-patched\` |
| \`gcr.io/distroless/static:latest\` | \`ghcr.io/verity-org/distroless/static:latest-patched\` |

For Wolfi-based images (no upstream equivalent):
\`\`\`
ghcr.io/verity-org/golang:latest
ghcr.io/verity-org/python:latest-dev
ghcr.io/verity-org/nginx:latest-fips
\`\`\`

On subsequent re-patches, the suffix increments: \`-patched-2\`, \`-patched-3\`, etc.

---

## How It Works

\`\`\`
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
\`\`\`

This pipeline runs daily at 02:00 UTC and on every \`copa-config.yaml\` change.

---

## Image Catalog

Verity maintains ${totalImages} container images across ${totalCategories} categories. All images are published to \`${REGISTRY}\`.

**Legend:**
- **Wolfi-based** = Built from scratch with minimal attack surface (no shell, no package manager)
- **Copa-patched** = Upstream image with OS packages patched in-place
- **Variants**: \`default\` (standard), \`dev\` (includes build tools/shell), \`fips\` (FIPS 140-2 compliant)

${catalogSections}

---

## Helm Charts

Verity publishes pre-patched wrapper Helm charts that override upstream image references with security-patched equivalents. Install the wrapper chart instead of the original — Helm resolves the dependency and applies all patched image overrides automatically.

### How Wrapper Charts Work

1. **Wrapper chart** — A thin Helm chart that declares the original chart as a dependency and overrides \`values.yaml\` to point image references at patched versions.
2. **OCI registry** — Wrapper charts are pushed to an OCI registry and can be installed directly via \`helm install\`.
3. **Drop-in replace** — Install the wrapper chart instead of the original. Helm resolves the dependency and applies all patched image overrides automatically.

${chartsSection}

---

## Supply Chain Compliance

Every patched image published by Verity carries five supply-chain attestations:

1. **cosign signature** — Keyless OIDC via GitHub Actions workflow identity (Sigstore/Fulcio)
2. **SLSA Level 3 build provenance** — Platform-generated by \`actions/attest-build-provenance\`, outside the control of the build definition
3. **CycloneDX SBOM** — Full package inventory generated by Trivy, attached via \`actions/attest-sbom\`
4. **Vulnerability report attestation** — Trivy scan results as in-toto attestation
5. **Rekor transparency log entry** — Tamper-evident, publicly auditable record of all signing events

### Signing Identity

All signatures can be verified against:
- **Issuer**: \`https://token.actions.githubusercontent.com\`
- **Identity**: \`https://github.com/verity-org/verity/.github/workflows/\`

### SLSA Level 3 — Build Integrity

| Level | Requirement | How Verity Meets It |
|-------|-------------|---------------------|
| Build L1 | Provenance Exists | Every patched image has SLSA provenance attestation generated at build time |
| Build L2 | Hosted Build Platform | All builds on GitHub Actions ephemeral runners, no self-hosted infrastructure |
| Build L3 | Hardened Builds | Provenance generated by \`actions/attest-build-provenance\`, outside build definition control |

### Sigstore — Artifact Signing

- **Keyless signing**: cosign with OIDC — GitHub Actions workflow identity token via Fulcio, no long-lived keys
- **Transparency log**: Every signature recorded in Rekor, tamper-evident and publicly auditable
- **Verification**: Signatures tied to specific GitHub Actions workflow runs

### Software Bill of Materials (SBOM)

- **Format**: CycloneDX
- **Generator**: Trivy
- **Attachment**: In-toto attestation via \`actions/attest-sbom\`
- **Storage**: GitHub attestation store + OCI registry (travels with the artifact)
- **Contents**: All OS packages, libraries, and versions for transitive risk assessment

### Compliance Framework Mapping

| Framework | Control | How Verity Meets It |
|-----------|---------|---------------------|
| **SLSA v1.0** | Build L3 — Hardened provenance | Platform-generated provenance via \`actions/attest-build-provenance\` on ephemeral runners |
| **SLSA v1.0** | Source integrity | Version-controlled manifests in Git; provenance links artifacts to commit SHAs |
| **EO 14028** | SBOM for delivered software | CycloneDX SBOM attested to every container image |
| **NIST SSDF** | PS.1 — Protect software integrity | Cosign keyless signatures; Rekor transparency log |
| **NIST SSDF** | PW.4 — Verify third-party components | Trivy scanning of all upstream images; Copa patching |
| **NIST SSDF** | RV.1 — Identify and confirm vulnerabilities | Daily scheduled scans; auto-generated PRs |
| **NIST 800-53r5** (FedRAMP) | SR-3/SR-4 — Supply chain provenance | SLSA L3 provenance linking artifacts to source, build, and runner |
| **NIST 800-53r5** (FedRAMP) | SR-11 — Component authenticity | Cosign OIDC-based signatures verify artifact origin |
| **NIST 800-53r5** (FedRAMP) | SI-7 — Software integrity | Signature verification detects post-build tampering |
| **NIST 800-53r5** (FedRAMP) | RA-5 — Vulnerability monitoring | Daily Trivy scans; 30-day report retention for audit evidence |
| **SOC 2** | CC8.1 — Change management | PR-gated changes; provenance ties artifacts to commits and runs |
| **SOC 2** | CC7.1 — Monitoring and detection | Daily CVE scans; vulnerability report artifacts retained |
| **SOC 2** | CC6.1 — Logical access controls | Scoped GITHUB_TOKEN; least-privilege permissions; no static keys |
| **SOC 2** | PI1.4 — Processing integrity | Cryptographic signatures and attestations verify artifact integrity |
| **ISO 27001:2022** | A.5.21 — ICT supply chain security | Pinned dependencies; SBOM attestation; continuous upstream scanning |
| **ISO 27001:2022** | A.8.25 — Secure development lifecycle | Automated CI gates (lint, test, scan) before any artifact is published |
| **ISO 27001:2022** | A.8.26 — Application security requirements | Vulnerability scanning enforces security on every container image |
| **ISO 27001:2022** | A.8.30 — Outsourced development | Third-party images scanned, patched, re-signed under Verity identity |
| **OWASP ASVS v4** | V14.2 — Dependency verification | All deps pinned by hash; Renovate auto-updates; no mutable tags |
| **OWASP ASVS v4** | V10.3 — Deployed application integrity | Cosign + \`gh attestation verify\` for end-to-end integrity |
| **OWASP ASVS v4** | V14.1 — Build & deploy | Scripted, reproducible builds on ephemeral CI runners; provenance attestations |
| **NIST CSF 2.0** | GV.SC — Supply chain risk management | Provenance, signing, SBOM, pinning, and scanning as unified posture |
| **CISA Secure by Design** | Radical transparency | Public vulnerability reports; SBOM on every image; Rekor for public audit |
| **OpenSSF Scorecard** | Pinned-Dependencies | Actions pinned by SHA; Go deps via go.sum; Renovate auto-updates |
| **OpenSSF Scorecard** | Signed-Releases | All OCI artifacts signed with cosign keyless |
| **Sigstore** | Artifact transparency | All signatures in Rekor transparency log; verifiable by anyone |

### Dependency Management

- **Go modules**: Pinned with cryptographic hashes in \`go.sum\`
- **GitHub Actions**: Pinned by full commit SHA, not floating tags
- **Automated updates**: Renovate monitors for dependency updates; security patches auto-merged

### Continuous Vulnerability Scanning

- **Daily scans**: Scheduled workflow at 02:00 UTC re-scans all images
- **Automatic patching**: Copa patches fixable OS-level vulnerabilities in-place
- **PR-driven updates**: New vulnerabilities trigger automatic pull requests with updated patched images

---

## Architecture

Verity is a Go CLI tool and a GitHub Actions pipeline.

### Components

| Component | Role |
|-----------|------|
| **Verity CLI** (Go) | Orchestrates scanning and catalog generation |
| **Copa** | Patches OS and application packages in container images without rebuilding |
| **Trivy** | Vulnerability scanner (CVE detection, SBOM generation) |
| **BuildKit** | Builds patched container images |
| **cosign** | Keyless image signing via Sigstore OIDC |
| **GitHub Actions** | CI/CD pipeline orchestration |

### Source Layout

\`\`\`
verity/
├── main.go                         CLI entry point (urfave/cli)
├── cmd/
│   ├── scan.go                     \`verity scan\` — parallel Trivy scanning
│   ├── catalog.go                  \`verity catalog\` — site data generation
│   ├── scan_test.go                Scan command tests
│   └── patch_test.go               Patch tag versioning tests
├── internal/
│   ├── copaconfig.go               copa-config.yaml parsing and image discovery
│   ├── copaconfig_test.go          Config parser tests
│   ├── sitedata.go                 Catalog JSON generation from Trivy reports
│   ├── sitedata_test.go            Catalog generation tests
│   └── types.go                    Image reference models and parsing
├── copa-config.yaml                Image/chart registry (the source of truth)
├── site/                           Astro static site (catalog + compliance)
│   ├── src/pages/                  index, compliance, image detail pages
│   ├── src/components/             UI components
│   ├── src/lib/                    TypeScript data models
│   └── src/data/catalog.json       Generated catalog data
└── .github/
    ├── workflows/
    │   ├── patch-matrix.yaml       Main pipeline (scan → patch → sign → publish)
    │   ├── ci.yaml                 Unit tests on PRs
    │   ├── lint.yaml               Code quality (8 linters)
    │   └── new-issue.yaml          Auto-PR from issue templates
    └── scripts/                    Shell helpers for workflow steps
\`\`\`

### Pipeline Stages (patch-matrix.yaml)

The main GitHub Actions workflow runs daily and on \`copa-config.yaml\` changes. Eight stages:

1. **mirror-buildkit** — Mirror BuildKit image to GHCR (avoids upstream flakiness)
2. **scan** — \`verity scan\` → Trivy reports; Copa dry-run → discovery matrix + skip detection
3. **patch** — Matrix job: one per image × platform (amd64, arm64); Copa patches via BuildKit
4. **combine** — Create multi-arch manifest lists; cosign signs each image (keyless OIDC)
5. **attest** — Attach CycloneDX SBOM + SLSA L3 build provenance attestations
6. **post-scan** — Trivy scans patched images; captures remaining (unfixable) vulnerabilities
7. **assemble** — \`verity catalog\` → catalog.json; before/after vulnerability metrics
8. **deploy-site** — Build Astro site → deploy to GitHub Pages

### PR Mode vs. Production Mode

On pull requests: uses local Docker registry (\`localhost:5000\`), skips signing/attestation/deployment, uploads test artifacts.
On push to main: full pipeline with signing, attestation, and deployment.

### Skip Detection

Copa checks whether the existing patched image already addresses all fixable vulnerabilities. If so, the image is skipped — avoiding unnecessary rebuilds and registry churn.

---

## CLI Reference

\`\`\`
verity - Self-maintaining registry of security-patched container images

Commands:
  scan        Scan images from copa-config.yaml and generate Trivy reports
  catalog     Generate site catalog JSON from patch reports
\`\`\`

### \`verity scan\`

Reads \`copa-config.yaml\`, resolves tags, and runs Trivy against each image in parallel.

\`\`\`bash
./verity scan \\
  --config copa-config.yaml \\
  --target-registry ghcr.io/verity-org \\
  --trivy-server http://localhost:4954 \\
  --parallel 10 \\
  --output reports/
\`\`\`

| Flag | Default | Description |
|------|---------|-------------|
| \`--config, -c\` | *(required)* | Path to \`copa-config.yaml\` |
| \`--output, -o\` | \`reports\` | Output directory for Trivy JSON reports |
| \`--parallel\` | \`5\` | Number of concurrent scans |
| \`--target-registry\` | | Registry to check for existing patched images |
| \`--trivy-server\` | | Trivy server address for client/server scanning |
| \`--patched-only\` | \`false\` | Scan only patched images (requires \`--target-registry\`) |

### \`verity catalog\`

Reads Trivy reports and an \`images.json\` manifest to produce \`catalog.json\`.

\`\`\`bash
./verity catalog \\
  --output site/src/data/catalog.json \\
  --images-json images.json \\
  --registry ghcr.io/verity-org \\
  --reports-dir reports/ \\
  --post-reports-dir post-reports/
\`\`\`

| Flag | Default | Description |
|------|---------|-------------|
| \`--output, -o\` | *(required)* | Output path for \`catalog.json\` |
| \`--images-json, -j\` | *(required)* | Path to \`images.json\` from patch run |
| \`--registry\` | | Target registry prefix for patched refs |
| \`--reports-dir\` | | Pre-patch Trivy report directory |
| \`--post-reports-dir\` | | Post-patch Trivy report directory |

---

## Configuration Format

The single source of truth for which images Verity monitors is \`copa-config.yaml\`:

\`\`\`yaml
apiVersion: copa.sh/v1alpha1
kind: PatchConfig

target:
  registry: "ghcr.io/verity-org"

# Helm chart — Copa auto-discovers all container images from templates
charts:
  - name: prometheus
    version: "28.9.1"
    repository: "oci://ghcr.io/prometheus-community/charts"

# Base-image override for images Copa can't patch directly
overrides:
  "timberio/vector":
    from: "distroless-libc"
    to: "debian"

# Standalone image with tag strategy
images:
  - name: "nginx"
    image: "mirror.gcr.io/library/nginx"
    platforms: ["linux/amd64", "linux/arm64"]
    tags:
      strategy: "pattern"
      pattern: '^\\d+\\.\\d+\\.\\d+$'
      maxTags: 3
\`\`\`

### Image Sources

- **Charts** — Copa renders Helm chart templates and auto-discovers every container image referenced
- **Images** — Standalone entries with explicit registry, platform, and tag strategy
- **Overrides** — Base-image substitutions for images Copa can't patch directly (e.g., distroless)

### Tag Strategies

| Strategy | Behavior |
|----------|----------|
| \`pattern\` | Regex filter on available tags. \`maxTags\` limits to the N most recent semver matches |
| \`latest\` | Resolves the latest semver tag from the registry |
| \`list\` | Explicit list of tags to patch |

---

## Verification Commands

Every patched image is signed and attested. Verify it yourself:

### Verify cosign signature

\`\`\`bash
cosign verify \\
  --certificate-identity-regexp "https://github.com/verity-org/verity/.github/workflows/" \\
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \\
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
\`\`\`

### Verify build provenance (GitHub CLI)

\`\`\`bash
gh attestation verify \\
  oci://ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched \\
  --owner verity-org
\`\`\`

---

## Contributing

### Development Setup

Prerequisites: **mise** (tool version manager), **Docker** (for Copa patching)

\`\`\`bash
git clone https://github.com/verity-org/verity.git
cd verity
mise install    # Installs: go, node, golangci-lint, gofumpt, govulncheck, gosec, etc.
make build      # Build the project
make test       # Run tests
make quality    # Run all quality checks
\`\`\`

### Quality Checks

\`\`\`bash
make fmt            # Format code
make lint           # Run golangci-lint
make vet            # Run go vet
make sec            # Run security scanner
make test           # Run tests
make lint-workflows # Lint GitHub Actions
make lint-yaml      # Lint YAML files
make lint-shell     # Lint shell scripts
\`\`\`

### Testing

\`\`\`bash
go test ./...                              # All tests
make test-coverage                         # With coverage report
RUN_INTEGRATION_TESTS=1 go test ./...      # Integration tests (requires OCI registry)
\`\`\`

### Adding an Image

1. **Via GitHub Issue**: Open an issue with the "Request New Image" template — Verity creates a PR automatically
2. **Via \`copa-config.yaml\`**: Add an entry under \`images:\` and create a PR

### Commit Convention

\`\`\`
feat: add new feature
fix: fix a bug
chore: update dependencies
docs: update documentation
test: add tests
refactor: refactor code
ci: update workflows
\`\`\`

---

## Links

- **Website**: ${siteUrl}
- **GitHub**: https://github.com/verity-org/verity
- **Issues**: https://github.com/verity-org/verity/issues
- **Discussions**: https://github.com/verity-org/verity/discussions
- **Copa**: https://github.com/project-copacetic/copacetic
- **Trivy**: https://github.com/aquasecurity/trivy
- **Sigstore**: https://www.sigstore.dev/
- **SLSA**: https://slsa.dev/
- **License**: MIT
`;

  return new Response(content.trim() + "\n", {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
