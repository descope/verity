# Architecture

Verity is a Go CLI tool and a suite of GitHub Actions workflows that continuously
scans, patches, rebuilds, signs, and publishes container images. This document
covers the system design, component responsibilities, and pipeline mechanics.

## Components

| Component | Role |
| --- | --- |
| **Verity CLI** (Go) | Orchestrates scanning, discovery, Integer builds, chart generation, patching, and catalog assembly |
| **Copa** | Go library (`github.com/project-copacetic/copacetic/pkg/patch`) bundled into the verity binary via `internal/patch`. Patches OS, Python (`pip`), and Go packages in container images without rebuilding. Exposed as `verity patch` |
| **Trivy** | Vulnerability scanner (CVE detection, SBOM generation) |
| **BuildKit** | Builds patched container images (production pipeline uses the GHCR-mirrored `buildx-stable-1` digest; PR smoke tests use a pinned buildx driver image; local `docker-compose.yaml` currently pins `moby/buildkit:v0.29.0`) |
| **apko / melange** | Builds Wolfi-based Integer images from source (apko rootfs + melange APKs) |
| **Helm** | Packages patched wrapper charts pushed to `oci://ghcr.io/verity-org/charts` |
| **cosign** | Keyless image signing via Sigstore OIDC |
| **GitHub Actions** | CI/CD pipeline orchestration (10 workflows) |

## Two Kinds of Images

Verity publishes two image families, both with the same supply-chain guarantees
currently produced by the workflows: cosign keyless signatures, an SBOM
attestation attached via [`actions/attest`](https://github.com/actions/attest),
and a corresponding Rekor transparency-log entry. The SBOM format differs by
family — Copa-patched images ship a CycloneDX SBOM produced by Syft, Integer
images ship an SPDX SBOM produced by apko.

| Family | What it is | How it's produced |
| --- | --- | --- |
| **Copa-patched** | Upstream image with OS + Python + Go packages patched in-place | `verity patch` (wraps Copa's Go library) via BuildKit — no Dockerfile rebuild |
| **Integer (Wolfi-based)** | From-scratch hardened rebuild using Wolfi packages with minimal attack surface | `apko` rootfs build + `melange` APK build from `images/*.yaml` |

FIPS variants are available for a curated set of images (`golang`, `nginx`,
`caddy`, `helm`, `terraform`, `cosign`, `crane`).

## Source Layout

```text
verity/
├── main.go                         CLI entry (urfave/cli/v3, version "2.0.0")
├── cmd/                            Top-level subcommands
│   ├── scan.go                     `verity scan` — parallel Trivy scanning
│   ├── catalog.go                  `verity catalog` — Copa site catalog JSON
│   ├── discover.go                 `verity discover` — enumerate image+tag combos
│   ├── preflight.go                `verity preflight update-manifest`
│   ├── chart_gen.go                `verity chart-gen` — Helm wrapper generation
│   ├── patch.go                    `verity patch` — single-image patch via Copa library
│   ├── integer.go                  `verity integer` (subcommand group)
│   ├── integer_{build,discover,sync,validate,catalog}.go
│   └── *_test.go
├── internal/
│   ├── chartgen/                   Helm wrapper chart generator
│   ├── config/                     Shared config types (CopaConfig, VerityConfig, …)
│   ├── discovery/                  Image discovery (Copa + Helm + verity.yaml)
│   ├── integer/                    Wolfi subsystem (apkindex, config, discovery, render)
│   ├── patch/                      Thin wrapper around Copa's pkg/patch.Patch library
│   ├── preflight/                  Preflight manifest for build skipping
│   ├── copaconfig.go               copa-config.yaml parsing
│   ├── sitedata.go                 Catalog JSON generation
│   └── types.go                    Image reference models
├── copa-config.yaml                Standalone image registry (Copa's domain)
├── Chart.yaml                      Helm chart dependencies (standard format)
├── verity.yaml                     Verity-specific overrides (tag variants + chart-gen replacements)
├── integer.yaml                    Integer/Wolfi build config
├── images/                         Wolfi melange configs (one `<name>.yaml` per image; 100+ images)
├── packages/
│   ├── bespoke/                    Bespoke melange package builds (crane, dive, ko, pgweb)
│   ├── overrides/fips.env          FIPS environment overrides
│   └── upstream.lock.json          Locked upstream package versions
├── site/                           Astro 6 static site
├── .github/workflows/              10 workflows (see Pipeline)
├── .github/scripts/                Workflow helper scripts
├── docker-compose.yaml             Local dev: registry on :5555, BuildKit v0.29.0
├── Makefile                        Dev targets incl. integer-validate, integer-gen
└── CONTRIBUTING.md / SECURITY.md / README.md
```

## CLI Reference

```text
verity - Self-maintaining registry of security-patched container images

Commands:
  scan        Scan images from copa-config.yaml and generate Trivy reports
  catalog     Generate site catalog JSON from patch reports
  discover    Enumerate image+tag combos from copa-config.yaml, Chart.yaml, and verity.yaml
  integer     Build and manage Wolfi-based OCI images from source (subcommand group)
  preflight   Manage preflight manifest for build skipping
  chart-gen   Generate and push patched wrapper Helm charts from Chart.yaml
  patch       Patch a single image via Copa (imported as a library)

Use "verity [command] --help" for command-specific options.
```

### `verity scan`

Reads `copa-config.yaml`, resolves tags using the configured strategy, and runs
Trivy against each image in parallel. Outputs one JSON report per image.

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
produce `catalog.json` — the data file consumed by the Astro site for Copa-patched
images.

| Flag | Default | Description |
| --- | --- | --- |
| `--output, -o` | *(required)* | Output path for `catalog.json` |
| `--images-json, -j` | *(required)* | Path to `images.json` from patch run |
| `--registry` | | Target registry prefix for patched refs |
| `--reports-dir` | | Pre-patch Trivy report directory |
| `--post-reports-dir` | | Post-patch Trivy report directory |

### `verity discover`

Enumerates every image+tag combination Verity is responsible for. Three sources
are merged: `copa-config.yaml` (standalone images), `Chart.yaml` (Helm chart
dependencies, rendered via `helm template`), and `verity.yaml` (tag variant
overrides). Output is a JSON array consumed by the orchestrator to fan out
per-image patch runs.

| Flag | Default | Description |
| --- | --- | --- |
| `--config, -c` | *(required)* | Path to `copa-config.yaml` |
| `--charts-file` | `Chart.yaml` | Helm Chart.yaml whose `dependencies:` provides chart images |
| `--verity-config` | `verity.yaml` | Tag variant overrides + chart-gen image replacements |
| `--target-registry` | | Override the target registry from config |
| `--only` | | Comma-separated list of image names to include (empty = all) |
| `--exclude-names` | | Comma-separated names to exclude (typically Integer/Wolfi names) |
| `--preflight` | `false` | Enable digest-based skip (compares upstream digests to manifest) |
| `--github-repo` | | Required when `--preflight` is enabled |

### `verity preflight update-manifest`

Updates the preflight manifest on the `reports` branch. The manifest tracks
upstream image digests and post-patch vulnerability counts (raw Trivy counts,
including unfixable CVEs) so the orchestrator can skip rebuilds that would
produce identical output. Requires `GH_TOKEN` or
`GITHUB_TOKEN` in the environment.

| Flag | Default | Description |
| --- | --- | --- |
| `--github-repo` | *(required)* | GitHub repository (`owner/repo`) |
| `--reports-branch` | `reports` | Branch where `preflight-manifest.json` is stored |
| `--image` | *(required)* | Image name |
| `--tag` | *(required)* | Image tag |
| `--upstream-digest` | | Upstream image digest (`sha256:…`) |
| `--patched-vulns` | `0` | Count of vulnerabilities remaining after patching. The CLI flag's help text calls these "fixable". This CLI is not currently invoked by the workflows — today `patch-image.yaml` updates `preflight-manifest.json` inline via `gh api` + `jq`, writing the raw Trivy post-patch count from `post.json` (no `--ignore-unfixed`, so the value includes unfixable CVEs) — but the CLI exists as a programmatic alternative |

### `verity integer`

Subcommand group for Wolfi-based Integer images — from-scratch hardened rebuilds
using apko + melange. Each image is defined by a melange YAML file under
`images/`.

| Subcommand | Purpose |
| --- | --- |
| `integer discover` | List all image+variant combos as JSON (CI matrix input) |
| `integer validate` | Schema-validate every file in `images/` against `integer.yaml` |
| `integer build` | Local single-arch apko build of one image variant |
| `integer sync` | Fetch Wolfi APKINDEX and report new/stale versions; `--apply` rewrites image files |
| `integer catalog` | Generate `catalog.json` for the Integer catalog on the site |

Common flags: `--config/-c` (path to `integer.yaml`, default `integer.yaml`),
`--images-dir` (default `images`), `--apkindex-url` (Wolfi APKINDEX URL).

### `verity chart-gen`

Generates patched wrapper Helm charts from `Chart.yaml`. For each dependency,
Verity produces a thin wrapper chart whose `values.yaml` overrides every image
reference to point at the patched equivalent in the target registry (Copa-patched
or Integer, according to `verity.yaml`). Wrappers are pushed as OCI artifacts.

| Flag | Default | Description |
| --- | --- | --- |
| `--charts-file` | `Chart.yaml` | Dependency source |
| `--verity-config` | `verity.yaml` | Tag variant overrides and value-path hints |
| `--target-registry` | *(required)* | Registry where patched images live (e.g., `ghcr.io/verity-org`) |
| `--chart-registry` | *(required)* | OCI registry for wrapper charts (e.g., `oci://ghcr.io/verity-org/charts`) |
| `--exclude-names` | | Comma-separated names to exclude |
| `--dry-run` | `false` | Output JSON plan without pushing charts |

### `verity patch`

Patches a single image via the Copa Go library. Copa is imported as a Go
dependency (`github.com/project-copacetic/copacetic/pkg/patch`) and called
directly — no separate `copa` binary is needed on the runner. This is the
Go-native replacement for the prior `copa patch` shell invocation; the CI
workflow `.github/scripts/patch-image.sh` calls this command.

| Flag | Default | Description |
| --- | --- | --- |
| `--image` | *(required)* | Source image reference (e.g. `mirror.gcr.io/library/nginx:1.29.3`) |
| `--tag` | *(required)* | Output tag for the patched image |
| `--report` | *(required)* | Path to Trivy JSON vulnerability report for `--image` |
| `--pkg-types` | `os,library` | Package ecosystems to patch (`os`, `library`, or `os,library`) |
| `--library-patch-level` | `patch` | Library version bump aggression (`patch`/`minor`/`major`) |
| `--toolchain-patch-level` | `patch` | Go toolchain upgrade aggression (`patch`/`minor`/`major`) |
| `--push` | `false` | Push the patched image to the registry |
| `--buildkit-addr` | *(empty)* | BuildKit endpoint (e.g., `buildx://copa-builder`) |
| `--timeout` | `5m` | Upper bound on the whole patch operation |
| `--platform` | *(empty)* | Single platform to build for (e.g. `linux/amd64`) |
| `--go-vcs-url` | *(empty)* | Go module VCS URL for stripped/distroless binaries; currently a no-op pending upstream copa PR #1546 |

Copa's sentinel `ErrNoUpdatesFound` maps to exit code `0` with stderr line
`"no package updates found for image <ref>"` so `patch-image.sh`'s existing
retry gate works unchanged.

## Configuration

Verity's behavior is driven by four YAML files at the repo root.

### `copa-config.yaml`

Copa's native config for standalone images and the catch-all overrides.

```yaml
apiVersion: copa.sh/v1alpha1
kind: PatchConfig

target:
  registry: "ghcr.io/verity-org"

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

  - name: "elastic/eck-operator"
    image: "mirror.gcr.io/elastic/eck-operator"
    platforms: ["linux/amd64", "linux/arm64"]
    tags:
      strategy: "pattern"
      pattern: '^\d+\.\d+\.\d+$'
      maxTags: 3
    goVcsUrl: "https://github.com/elastic/cloud-on-k8s"
    goVcsTagPrefix: "v"
```

Images declaring `goVcsUrl` trigger Copa's Go binary patching path. Around a
dozen images in `copa-config.yaml` currently use this (cert-manager components,
promtail, rabbitmq operators, VictoriaLogs, and others).

### `Chart.yaml`

Standard Helm `Chart.yaml` dependency format. Verity renders each chart via
`helm template` and patches every container image the templates reference.

```yaml
dependencies:
  - name: prometheus
    version: "29.2.1"
    repository: "oci://ghcr.io/prometheus-community/charts"
  - name: victoria-logs-single
    version: "0.12.0"
    repository: "https://victoriametrics.github.io/helm-charts"
  - name: postgres-operator
    version: "1.15.1"
    repository: "https://opensource.zalando.com/postgres-operator/charts/postgres-operator"
```

### `verity.yaml`

Verity-specific settings that don't belong in Copa's config or Helm's Chart.yaml:

- **Tag variant overrides** — e.g., replace `-fips` upstream tags with
  `-fips-patched` in the target.
- **Image replacements for chart-gen** — map upstream image paths to Verity
  Integer (Wolfi) equivalents so wrapper charts route to the hardened rebuild
  instead of the Copa-patched upstream.

### `integer.yaml`

Top-level Integer build config (`apiVersion: integer.verity.supply/v1alpha1`).
Declares the target registry and default platforms; per-image detail lives in
`images/<name>.yaml` melange configs.

### Tag Strategies

| Strategy | Behavior |
| --- | --- |
| `pattern` | Regex filter on available tags; `maxTags` limits to the N most recent semver matches |
| `latest` | Resolves the latest semver tag from the registry |
| `list` | Explicit list of tags to patch |

### Image Naming

Source images are published under the target registry with a `-patched` suffix.
The registry prefix is stripped and replaced:

- **Source:** `quay.io/prometheus/prometheus:v3.9.1`
- **Patched:** `ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched`

On subsequent re-patches the suffix increments: `-patched-2`, `-patched-3`, etc.

## Pipeline

Ten GitHub Actions workflows cover patching, Wolfi rebuilds, chart generation,
site deployment, and PR validation. Nightly runs are scheduled in sequence:

```text
02:00 UTC — orchestrator.yaml            (Copa patching dispatcher)
03:00 UTC — integer-orchestrator.yaml    (Wolfi rebuild dispatcher)
04:00 UTC — chart-gen.yaml               (Helm wrapper generation)
05:00 UTC — build-site.yaml              (catalog assembly + site deploy)
```

### `orchestrator.yaml` — Copa dispatcher

Runs `verity discover` against `copa-config.yaml`, `Chart.yaml`, and
`verity.yaml`, then dispatches one `patch-image.yaml` run per image+tag.
Fire-and-forget — does NOT wait for per-image runs to complete. This keeps the
dispatcher fast and lets each image patch in its own isolated workflow with its
own logs and artifacts.

Triggers: nightly cron `0 2 * * *`, push to `main` touching
`copa-config.yaml`/`Chart.yaml`/`verity.yaml`, and `workflow_dispatch` with an
optional `image` input to patch a single image on demand.

### `patch-image.yaml` — reusable per-image lifecycle

```text
┌──────────┐   scan-before.sh: Pre-patch Trivy (or skip on preflight hit)
│   scan   │
└────┬─────┘
     ▼
┌──────────┐   Copa patches packages on matrix (linux/amd64, linux/arm64)
│  patch   │
└────┬─────┘
     ▼
┌──────────┐   Multi-arch manifest, cosign keyless sign, CycloneDX SBOM
│ finalize │   attestation (via actions/attest), push to target registry,
└──────────┘   push reports to `reports` branch, update preflight manifest
```

`patch-image.yaml` declares both `workflow_call` and `workflow_dispatch`
triggers; today it is only invoked via `workflow_dispatch` from
`orchestrator.yaml` for full production runs. PR validation runs its own
patching path inline — see `pr-test.yaml` below.

### `integer-orchestrator.yaml` + `integer-build-image.yaml`

The Integer dispatcher offsets itself one hour after Copa (03:00 UTC) to avoid
resource contention. It runs `verity integer discover` to build a matrix of
(image × version × variant) combinations, then dispatches
`integer-build-image.yaml` per entry. Each per-image build runs apko + melange,
pushes to the target registry, and captures a post-build Trivy scan.

### `chart-gen.yaml`

Runs at 04:00 UTC (after both Copa and Integer have had time to settle). Calls
`verity chart-gen` to generate one wrapper chart per dependency in `Chart.yaml`
and pushes to `oci://ghcr.io/verity-org/charts`. Each wrapper's `values.yaml`
overrides upstream image references to point at the patched (or Integer)
equivalent.

### `build-site.yaml`

Runs at 05:00 UTC, fully decoupled from patching. Assembles `catalog.json` from
three independent sources:

1. `copa-config.yaml` via `verity discover` — source of truth for intended images
2. The `reports` branch — pre/post Trivy reports written by `patch-image.yaml`
3. The registry (via `crane`) — confirms patched images are actually published

Then builds the Astro site and deploys to GitHub Pages. Running catalog
assembly on its own schedule means the site always reflects the registry's
actual state, even if individual patch runs failed or haven't completed yet.

### `pr-test.yaml` — lightweight PR validation

Pull requests do NOT run the full nightly orchestration or `patch-image.yaml`.
Instead, `pr-test.yaml` is standalone and runs:

- `verity discover` — validates config syntax end-to-end
- Integer config validation (`verity integer validate`) and smoke builds via
  `melange-check.sh` + `melange-build.sh`
- For images changed in the PR, an inline Copa patch pass using
  `.github/scripts/patch-image.sh` (single-arch, typically `linux/amd64`)
  against a local/cache registry — signing, SBOM attestation, multi-arch
  manifest creation, and reports-branch push are all skipped

This keeps PR feedback fast while still exercising the real patch path. See
[.github/PR-TESTING.md](.github/PR-TESTING.md) for details.

### Remaining workflows

| Workflow | Purpose |
| --- | --- |
| `ci.yaml` | Go unit tests on every PR |
| `lint.yaml` | Code quality (`golangci-lint`, `shellcheck`, `yamllint`, `actionlint`, `markdownlint`, `gosec`, `govulncheck`) |
| `new-issue.yaml` | Parses `new-image` issue form; opens PR that adds the entry to `copa-config.yaml` |

### Skip Detection (Preflight)

`verity preflight` maintains a manifest on the `reports` branch that records
each published image's upstream digest and post-patch vulnerability count
(raw Trivy count from `post.json` — includes unfixable CVEs, since the pipeline
does not pass `--ignore-unfixed`). When `verity discover --preflight` runs,
images whose upstream digest hasn't changed AND whose recorded vulnerability
count is zero are skipped — avoiding unnecessary rebuilds, registry churn,
and signing traffic.

## Site Architecture

The catalog site is an Astro 6 static site (Tailwind 4) deployed to GitHub
Pages. `catalog.json` + `integer-catalog.json` drive every page.

| Page | Source | Content |
| --- | --- | --- |
| Home | `pages/index.astro` | HeroSection, top 3 Helm charts, searchable image catalog |
| Charts | `pages/charts/index.astro` | Full listing of patched Helm wrapper charts + override counts per chart |
| Catalog detail | `pages/catalog/[...name].astro` | Per-image page: Copa variant, Integer variant, vulnerability breakdown, supply-chain badges |
| Image detail | `pages/images/[id].astro` | Legacy Copa-patched image detail |
| Compliance | `pages/compliance.astro` | Framework mappings (SLSA, FedRAMP, SOC 2, ISO 27001, OWASP, NIST CSF 2.0, CISA Secure by Design) |
| `llms.txt` | `pages/llms.txt.ts` | LLM-targeted sitemap and overview |
| `llms-full.txt` | `pages/llms-full.txt.ts` | Complete documentation dump for LLM consumption |
| `index.md` / `compliance.md` / `charts/index.md` | `*.md.ts` | Markdown variants of the main pages for agent consumption |

## Automation

### Daily Scans

Cron triggers cascade across the night: Copa at 02:00 UTC, Integer at 03:00,
chart-gen at 04:00, site build at 05:00.

For **Copa-patched images**, whether an image actually re-publishes on a given
run depends on the skip checks (see Skip Detection above) plus whether the
post-patch Trivy vulnerability-ID set has changed since the previous
published report on the `reports` branch — a change, including the appearance
of a previously-unseen unfixable CVE, triggers a new push.

For **Integer (Wolfi) images**, the build runs on a schedule or when
`images/<name>.yaml` changes; there is no vuln-ID-set delta comparison, so
rebuilt images are published whenever the build succeeds.

### Dependency Updates (Renovate)

Renovate monitors Go modules, GitHub Actions versions, and tool versions in
`mise.toml`. Security patches auto-merge. See
[.github/RENOVATE.md](.github/RENOVATE.md).

### New Image Requests

GitHub Issues with the `new-image` label trigger `new-issue.yaml`, which parses
the issue form, adds the image to `copa-config.yaml`, and opens a PR.

## Security Model

Every published image — Copa-patched or Integer — currently carries:

1. **cosign signature** — Keyless OIDC via GitHub Actions workflow identity
2. **SBOM attestation** — Produced via [`actions/attest`](https://github.com/actions/attest)
   as an in-toto attestation. Copa-patched images ship a CycloneDX SBOM generated
   by Syft (in `patch-image.yaml`); Integer images ship an SPDX SBOM produced by
   apko (in `integer-build-image.yaml`).
3. **Rekor transparency-log entry** — Tamper-evident, publicly auditable, recorded
   automatically by cosign keyless and `actions/attest`.

The signing identity is scoped to the Verity repository workflow:
`https://github.com/verity-org/verity/.github/workflows/` issued by
`https://token.actions.githubusercontent.com`.

> Historical note: earlier iterations of this documentation also advertised
> SLSA L3 build provenance and a Trivy vulnerability-report attestation. Those
> steps are not yet wired into `patch-image.yaml` / `integer-build-image.yaml`
> today; if you need them, track (or open) the follow-up issue to add them.

Copa-patched images never modify the upstream application layer beyond updating
vulnerable packages (OS, Python, and Go binaries via `goVcsUrl`), preserving
original image behavior. Integer images are rebuilt from Wolfi source — the
application layer may differ from upstream but the tradeoff is a minimal
attack-surface base with every package built deterministically from source.
