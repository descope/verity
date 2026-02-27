# Architecture

Verity is a Go CLI tool and a GitHub Actions pipeline that continuously scans,
patches, signs, and publishes container images. This document covers the system
design, component responsibilities, and pipeline mechanics.

## Components

| Component | Role |
| --- | --- |
| **Verity CLI** (Go) | Orchestrates scanning and catalog generation |
| **Copa** | Patches OS and application packages in container images without rebuilding |
| **Trivy** | Vulnerability scanner (CVE detection, SBOM generation) |
| **BuildKit** | Builds patched container images |
| **cosign** | Keyless image signing via Sigstore OIDC |
| **GitHub Actions** | CI/CD pipeline orchestration |

## Source Layout

```text
verity/
├── main.go                         CLI entry point (urfave/cli)
├── cmd/
│   ├── scan.go                     `verity scan` — parallel Trivy scanning
│   ├── catalog.go                  `verity catalog` — site data generation
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
├── .github/
│   ├── workflows/
│   │   ├── patch-matrix.yaml       Main pipeline (scan → patch → sign → publish)
│   │   ├── ci.yaml                 Unit tests on PRs
│   │   ├── lint.yaml               Code quality (8 linters)
│   │   └── new-issue.yaml          Auto-PR from issue templates
│   └── scripts/                    Shell helpers for workflow steps
├── Makefile                        Local development targets
└── CONTRIBUTING.md                 Development guide
```

## CLI Reference

```text
verity - Self-maintaining registry of security-patched container images

Commands:
  scan        Scan images from copa-config.yaml and generate Trivy reports
  catalog     Generate site catalog JSON from patch reports

Use "verity [command] --help" for command-specific options.
```

### `verity scan`

Reads `copa-config.yaml`, resolves tags using the configured strategy, and runs
Trivy against each image in parallel. Outputs one JSON report per image.

```bash
./verity scan \
  --config copa-config.yaml \
  --target-registry ghcr.io/verity-org \
  --trivy-server http://localhost:4954 \
  --parallel 10 \
  --output reports/
```

| Flag | Default | Description |
| --- | --- | --- |
| `--config, -c` | *(required)* | Path to `copa-config.yaml` |
| `--output, -o` | `reports` | Output directory for Trivy JSON reports |
| `--parallel` | `5` | Number of concurrent scans |
| `--target-registry` | | Registry to check for existing patched images |
| `--trivy-server` | | Trivy server address for client/server scanning |
| `--patched-only` | `false` | Scan only patched images (requires `--target-registry`) |

### `verity catalog`

Reads Trivy reports (pre-patch and post-patch) and an `images.json` manifest to
produce `catalog.json` — the data file consumed by the Astro site.

```bash
./verity catalog \
  --output site/src/data/catalog.json \
  --images-json images.json \
  --registry ghcr.io/verity-org \
  --reports-dir reports/ \
  --post-reports-dir post-reports/
```

| Flag | Default | Description |
| --- | --- | --- |
| `--output, -o` | *(required)* | Output path for `catalog.json` |
| `--images-json, -j` | *(required)* | Path to `images.json` from patch run |
| `--registry` | | Target registry prefix for patched refs |
| `--reports-dir` | | Pre-patch Trivy report directory |
| `--post-reports-dir` | | Post-patch Trivy report directory |

## Configuration: `copa-config.yaml`

The single source of truth for which images Verity monitors.

```yaml
apiVersion: copa.sh/v1alpha1
kind: PatchConfig

target:
  registry: "ghcr.io/verity-org"

charts:
  - name: prometheus
    version: "28.9.1"
    repository: "oci://ghcr.io/prometheus-community/charts"

overrides:
  "timberio/vector":
    from: "distroless-libc"
    to: "debian"

images:
  - name: "nginx"
    image: "mirror.gcr.io/library/nginx"
    platforms: ["linux/amd64", "linux/arm64"]
    tags:
      strategy: "pattern"
      pattern: '^\d+\.\d+\.\d+$'
      maxTags: 3
```

### Image Sources

**Charts** — Copa renders Helm chart templates and auto-discovers every
container image referenced. No manual image enumeration needed.

**Images** — Standalone image entries with explicit registry, platform, and tag
strategy configuration.

**Overrides** — Base-image substitutions for images Copa can't patch directly
(e.g., distroless). Copa patches the substitute and maps it back.

### Tag Strategies

| Strategy | Behavior |
| --- | --- |
| `pattern` | Regex filter on available tags. `maxTags` limits to the N most recent semver matches. |
| `latest` | Resolves the latest semver tag from the registry. |
| `list` | Explicit list of tags to patch. |

### Image Naming

Source images are published under the target registry with a `-patched` suffix.
The registry prefix is stripped and replaced:

- **Source:** `quay.io/prometheus/prometheus:v3.9.1`
- **Patched:** `ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched`

On subsequent re-patches, the suffix increments: `-patched-2`, `-patched-3`, etc.

## Pipeline: `patch-matrix.yaml`

The main GitHub Actions workflow runs daily and on `copa-config.yaml` changes.
It has eight stages:

```text
┌────────────────┐
│ mirror-buildkit │  Mirror BuildKit image to GHCR (avoids upstream flakiness)
└───────┬────────┘
        ▼
┌────────────────┐
│     scan       │  verity scan → Trivy reports
│                │  Copa dry-run → discovery matrix + skip detection
└───────┬────────┘
        ▼
┌────────────────┐
│     patch      │  Matrix job: one per image × platform (amd64, arm64)
│                │  Copa patches packages via BuildKit
└───────┬────────┘
        ▼
┌────────────────┐
│    combine     │  Create multi-arch manifest lists
│                │  cosign signs each image (keyless OIDC)
└───────┬────────┘
        ▼
┌────────────────┐
│     attest     │  Attach CycloneDX SBOM attestations
│                │  Attach SLSA L3 build provenance
└───────┬────────┘
        ▼
┌────────────────┐
│   post-scan    │  Trivy scans patched images
│                │  Captures remaining (unfixable) vulnerabilities
└───────┬────────┘
        ▼
┌────────────────┐
│    assemble    │  verity catalog → catalog.json
│                │  Before/after vulnerability metrics
└───────┬────────┘
        ▼
┌────────────────┐
│  deploy-site   │  Build Astro site → deploy to GitHub Pages
└────────────────┘
```

### PR Mode vs. Production Mode

On pull requests the pipeline validates the config without publishing:

- Uses a local Docker registry (`localhost:5000`) instead of GHCR
- Skips signing, attestation, and site deployment
- Uploads test artifacts (images.json, reports, catalog) for review

On push to `main`, the full pipeline runs with signing, attestation, and
deployment. See [.github/PR-TESTING.md](.github/PR-TESTING.md) for details.

### Skip Detection

Copa checks whether the existing patched image already addresses all fixable
vulnerabilities. If so, the image is skipped — avoiding unnecessary rebuilds
and registry churn.

## Site Architecture

The catalog site is an Astro static site deployed to GitHub Pages.

**Data flow:** `catalog.json` (generated by `verity catalog`) drives all pages.

| Page | Source | Content |
| --- | --- | --- |
| Home | `pages/index.astro` | Stats dashboard, searchable image catalog |
| Image detail | `pages/images/[id].astro` | Patched ref, supply chain badges, vulnerability breakdown |
| Compliance | `pages/compliance.astro` | Framework mappings (SLSA, FedRAMP, SOC 2, ISO 27001, OWASP) |

## Automation

### Daily Scans

A cron trigger at 02:00 UTC runs the full pipeline. If new fixable
vulnerabilities are found, images are patched and published automatically.

### Dependency Updates (Renovate)

Renovate monitors Go modules, GitHub Actions versions, and tool versions in
`mise.toml`. Security patches auto-merge. See [.github/RENOVATE.md](.github/RENOVATE.md).

### New Image Requests

GitHub Issues with the `new-image` label trigger `new-issue.yaml`, which parses
the issue form, adds the image to `copa-config.yaml`, and opens a PR.

## Security Model

Every patched image carries:

1. **cosign signature** — Keyless OIDC via GitHub Actions workflow identity
2. **SLSA L3 provenance** — Platform-generated, outside the build's control
3. **CycloneDX SBOM** — Full package inventory generated by Trivy
4. **Vulnerability report attestation** — Trivy scan results as in-toto attestation
5. **Rekor transparency log entry** — Tamper-evident, publicly auditable

The signing identity is scoped to the Verity repository workflow:
`https://github.com/verity-org/verity/.github/workflows/` issued by
`https://token.actions.githubusercontent.com`.

Patched images never modify the upstream application layer beyond updating
vulnerable packages (OS-level and pip), preserving the original image's behavior.
