# Verity

**Self-maintaining registry of security-patched Helm charts**

Verity automatically scans Helm chart dependencies for container image vulnerabilities, patches them using [Copa](https://github.com/project-copacetic/copacetic), and publishes wrapper charts that make it easy to consume patched images while maintaining full chart customization.

## Quick Start

### Install a Patched Chart

```bash
# Install prometheus with security-patched images
helm install my-prometheus \
  oci://ghcr.io/descope/charts/prometheus-verity \
  --version 1.0.0

# With custom values (patched images automatically included)
helm install my-prometheus \
  oci://ghcr.io/descope/charts/prometheus-verity \
  -f my-values.yaml
```

### Run Locally

```bash
# Scan charts for images
./verity -chart Chart.yaml -output charts

# Scan and patch with Copa
./verity -chart Chart.yaml -output charts \
  -patch \
  -registry ghcr.io/your-org \
  -buildkit-addr docker-container://buildkitd
```

## How It Works

```
Chart.yaml Dependencies
        ↓
  Scan for Images (verity)
        ↓
  Vulnerability Scan (Trivy)
        ↓
  Patch Images (Copa)
        ↓
  Wrapper Charts Created
        ↓
  Published to GHCR
```

### What Gets Created

For each chart dependency, verity creates a wrapper chart:

```
charts/
  prometheus-verity/
    Chart.yaml    # Depends on original prometheus chart
    values.yaml   # Patched images (namespaced)
    .helmignore
```

**Example values.yaml:**
```yaml
prometheus:
  server:
    image:
      registry: ghcr.io/descope
      repository: prometheus
      tag: v2.48.0-patched
  alertmanager:
    image:
      registry: ghcr.io/descope
      repository: alertmanager
      tag: v0.26.0-patched
```

## Automation

Verity is **fully automated** with GitHub Actions:

### 1️⃣ Scheduled Scans (Daily)
- Scans for new vulnerabilities
- Creates PR if patches available
- Runs daily at 2 AM UTC

### 2️⃣ Auto-Patch on Updates (Renovate)
- Renovate bumps chart version
- Workflow auto-patches images
- Commits to same PR
- Ready to merge!

### 3️⃣ Publish to GHCR (On Merge)
- Wrapper charts published to OCI registry
- Patched images verified
- Chart index generated

See [WORKFLOWS.md](WORKFLOWS.md) for details.

## Benefits

### For Chart Maintainers
✅ Security patches without forking upstream
✅ Update chart versions independently
✅ Automated vulnerability monitoring
✅ Publish to your own registry

### For Chart Consumers
✅ Drop-in replacements for original charts
✅ All customization options preserved
✅ Transparent security patching
✅ Zero-trust supply chain (verify patches yourself)

## Architecture

### Components

- **Verity** (Go) - Chart scanner and wrapper generator
- **Trivy** - Vulnerability scanner
- **Copa** - Microsoft's container patching tool
- **BuildKit** - Image building
- **Helm** - Chart dependency management

### Workflow System

```
┌──────────────┐
│  Renovate    │ Updates Chart.yaml
└──────┬───────┘
       ↓
┌──────────────────┐
│ patch-on-pr.yaml │ Auto-patches
└──────┬───────────┘
       ↓
┌────────────────┐
│ Merge to main  │
└──────┬─────────┘
       ↓
┌─────────────┐
│ publish.yaml│ Pushes to GHCR
└─────────────┘
```

Plus scheduled scans for continuous monitoring.

## Usage

### Add Charts to Monitor

Edit `Chart.yaml`:
```yaml
dependencies:
  - name: prometheus
    version: "25.8.0"
    repository: oci://ghcr.io/prometheus-community/charts
  - name: grafana
    version: "7.0.0"
    repository: https://grafana.github.io/helm-charts
```

Renovate and workflows handle the rest.

### Configuration

**Registry:**
Set via `-registry` flag or let it default to `ghcr.io/<org>`.

**Scan Schedule:**
Edit `.github/workflows/scheduled-scan.yaml`:
```yaml
schedule:
  - cron: '0 2 * * *'  # Daily at 2 AM UTC
```

**Renovate:**
See `.github/renovate.json` for dependency update config.

## Installation

### Prerequisites

- Go 1.24+
- Docker
- Helm 3
- BuildKit (for patching)

### Build

```bash
go build -o verity .
```

### Docker

```bash
docker run --rm -v $(pwd):/workspace \
  ghcr.io/descope/verity:latest \
  -chart /workspace/Chart.yaml -output /workspace/charts
```

## CLI Reference

```
verity [options]

Options:
  -chart string
        Path to Chart.yaml (default "Chart.yaml")
  -output string
        Output directory for charts (default "charts")
  -patch
        Enable patching with Trivy + Copa
  -registry string
        Target registry for patched images (e.g. ghcr.io/org)
  -buildkit-addr string
        BuildKit address (e.g. docker-container://buildkitd)
  -report-dir string
        Directory for Trivy JSON reports
```

**Examples:**

```bash
# Scan only
./verity -chart Chart.yaml -output ./charts

# Scan and patch
./verity -chart Chart.yaml -output ./charts \
  -patch \
  -registry ghcr.io/myorg \
  -buildkit-addr docker-container://buildkitd

# With custom report directory
./verity -chart Chart.yaml -output ./charts \
  -patch \
  -registry ghcr.io/myorg \
  -buildkit-addr docker-container://buildkitd \
  -report-dir ./reports
```

## Development

### Run Tests

```bash
go test ./...
```

### Validate Workflows

```bash
# Check YAML syntax
for f in .github/workflows/*.yaml; do
  yq eval '.' "$f" > /dev/null && echo "✅ $f" || echo "❌ $f"
done
```

### Local Testing

```bash
# Start BuildKit
docker run -d --privileged --name buildkitd \
  moby/buildkit:v0.19.0

# Run verity with patching
./verity -chart Chart.yaml -output /tmp/test-charts \
  -patch \
  -registry ghcr.io/test \
  -buildkit-addr docker-container://buildkitd

# Cleanup
docker stop buildkitd && docker rm buildkitd
```

## Documentation

- [WORKFLOWS.md](WORKFLOWS.md) - Complete workflow automation guide
- [WRAPPER_CHARTS.md](WRAPPER_CHARTS.md) - How wrapper charts work
- [.github/RENOVATE.md](.github/RENOVATE.md) - Renovate integration

## Security

### Vulnerability Scanning

Every patch run includes:
- Trivy JSON reports (attached to workflow runs)
- CVE details and CVSS scores
- Fixable vs unfixable vulnerabilities

### Image Trust

Patched images are:
1. Built from official upstream images
2. Scanned with Trivy (open source)
3. Patched with Copa (Microsoft, open source)
4. Pushed to your registry with `-patched` suffix
5. Never modify upstream images

### Supply Chain

Verify patches yourself:
```bash
# Pull patched image
docker pull ghcr.io/descope/prometheus:v2.48.0-patched

# Compare to original
docker pull quay.io/prometheus/prometheus:v2.48.0
docker diff <original-id> <patched-id>
```

## FAQ

**Q: What types of vulnerabilities can Copa patch?**
A: Copa patches OS-level packages (apt, yum, apk). It cannot patch application vulnerabilities in compiled binaries.

**Q: Will this patch ALL vulnerabilities?**
A: No. Only vulnerabilities with available package updates. Some images may have unfixable CVEs.

**Q: Can I use my existing Chart values?**
A: Yes! Wrapper charts support all original chart values. Just namespace them under the chart name (or use them as-is, Helm handles it).

**Q: What if I don't want to auto-merge security updates?**
A: Edit `.github/renovate.json` and set `automerge: false` for vulnerability alerts.

**Q: How do I add more charts?**
A: Just add them to `Chart.yaml` dependencies. Workflows automatically handle any number of charts.

**Q: Can I run this without GitHub Actions?**
A: Yes! Verity is a standalone CLI tool. Run it manually or integrate with any CI system.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Open a pull request

## License

[MIT License](LICENSE)

## Acknowledgments

- [Copa](https://github.com/project-copacetic/copacetic) - Microsoft's container patching tool
- [Trivy](https://github.com/aquasecurity/trivy) - Vulnerability scanner
- [Helm](https://helm.sh) - Kubernetes package manager
- [Renovate](https://renovatebot.com) - Dependency automation

---

**Built with ❤️ to make Kubernetes more secure**
