# Contributing to Verity

Thank you for contributing to Verity! This guide will help you set up your development environment and understand
our quality standards.

## Development Setup

### Prerequisites

- **mise**: Tool version manager (recommended - installs everything)
  - Install: <https://mise.jdx.dev>
- **Docker**: Required for Copa patching (runtime dependency)

### Quick Start

```bash
# Clone the repository
git clone https://github.com/verity-org/verity.git
cd verity

# Install ALL tools via mise (recommended)
mise install
# Installs: go, node, golangci-lint, gofumpt, govulncheck,
#           gosec, goimports, actionlint, shellcheck, yamllint,
#           markdownlint, crane, claude-code

# Or use Makefile shortcut (also requires mise)
make install-tools

# Build the project
make build

# Run tests
make test
```

## Code Quality

We maintain high code quality standards using automated tools.

### Running Quality Checks Locally

```bash
# Run all quality checks (recommended before committing)
make quality  # lint + lint-vuln + lint-workflows + lint-yaml + lint-shell +
              # lint-markdown + check-frontend + integer-validate + test

# Or run individual checks:
make fmt           # Format code
make lint          # Run golangci-lint
make vet           # Run go vet
make sec           # Run security scanner
make test          # Run tests
make lint-workflows  # Lint GitHub Actions
make lint-yaml     # Lint YAML files
make lint-shell    # Lint shell scripts
make integer-validate  # Validate Integer image configs
```

### Pre-commit Hooks (Recommended)

Install pre-commit hooks to automatically check code before committing:

```bash
# Install pre-commit (if not already installed)
pip install pre-commit
# or
brew install pre-commit

# Install the git hooks
pre-commit install

# Run hooks manually
pre-commit run --all-files
```

### Linters Configuration

- **golangci-lint**: `.golangci.yml` - Go code linting
- **yamllint**: `.yamllint.yml` - YAML file linting
- **actionlint**: Runs on GitHub Actions workflows
- **shellcheck**: Lints bash scripts in `.github/scripts/`

## Testing

### Running Tests

```bash
# All tests
go test ./...

# With coverage
make test-coverage
# Opens coverage.html in browser

# Specific package
go test ./internal

# Integration tests (requires OCI registry access)
RUN_INTEGRATION_TESTS=1 go test ./...
```

### Writing Tests

- Place tests in `*_test.go` files
- Use table-driven tests for multiple cases
- Test edge cases (empty inputs, missing files, nil values)
- Add integration tests for OCI interactions (mark with skip check)

## Pull Request Guidelines

### Before Submitting

1. ✅ Run `make quality` - all checks must pass
2. ✅ Add tests for new functionality
3. ✅ Update documentation if needed
4. ✅ Ensure CI passes on your branch

### PR Description

Include:

- **Problem**: What issue does this solve?
- **Solution**: How does it solve it?
- **Testing**: How did you test the changes?
- **Breaking Changes**: Any API or behavior changes?

### Commit Messages

Follow conventional commits:

```text
feat: add new feature
fix: fix a bug
chore: update dependencies
docs: update documentation
test: add tests
refactor: refactor code
ci: update workflows
```

## Architecture Overview

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design.

### Workflows

Patching pipeline (production):

- **orchestrator.yaml**: Nightly Copa dispatcher (02:00 UTC)
- **patch-image.yaml**: Reusable per-image scan/patch/sign
- **integer-orchestrator.yaml**: Nightly Wolfi rebuild dispatcher (03:00)
- **integer-build-image.yaml**: Per-image Wolfi build
- **chart-gen.yaml**: Helm wrapper generation (04:00)
- **build-site.yaml**: Catalog assembly + site deploy (05:00)

The main patching pipeline (`orchestrator.yaml` + `patch-image.yaml`) is
production-only — it runs nightly and on `main` pushes that touch
`copa-config.yaml` / `Chart.yaml` / `verity.yaml`, and via
`workflow_dispatch`. Pull requests use the standalone `pr-test.yaml` workflow,
which does its own inline Copa patch pass without signing or publishing.

Validation:

- **ci.yaml**: Go unit tests on PRs
- **pr-test.yaml**: Lightweight PR validation — `verity discover`, Integer
  config validation and smoke builds, plus an inline Copa patch pass on
  changed images (single-arch, no signing/attestation/publish)
- **lint.yaml**: Code quality (8 linters)

Automation:

- **new-issue.yaml**: Auto-PR from "new-image" issue template

### Key Components

**cmd/** — CLI commands (urfave/cli/v3)

- `scan.go`: Parallel Trivy scanning
- `catalog.go`: Copa site catalog JSON
- `discover.go`: Enumerate image+tag combos from 3 sources
- `preflight.go`: Preflight manifest for build skipping
- `chart_gen.go`: Helm wrapper chart generation
- `integer.go`: Subcommand group
- `integer_{build,discover,sync,validate,catalog}.go`

**internal/** — Core logic

- `copaconfig.go`: copa-config.yaml parsing
- `sitedata.go`: Catalog JSON generation
- `types.go`: Image reference models
- `chartgen/`: Helm wrapper chart generator
- `config/`: Shared config types
- `discovery/`: Image discovery
- `integer/`: Wolfi subsystem (apkindex, config, discovery, render)
- `preflight/`: Preflight manifest management

**images/**: Wolfi melange configs (100+ *.yaml files)  
**packages/**: Bespoke melange packages + FIPS overrides  
**.github/scripts/**: Workflow helper scripts  
**site/**: Astro static site

## Local Testing

Test patching without touching external registries:

```bash
# Start local registry + BuildKit
make up

# Scan images to generate Trivy reports
./verity scan --config copa-config.yaml --output reports/

# Patch a single image with local registry (Copa handles patching)
copa patch \
  --image "docker.io/library/nginx:1.29.5" \
  --report "reports/docker.io_library_nginx_1.29.5.json" \
  --tag "localhost:5555/verity/nginx:1.29.5-patched" \
  --addr "tcp://localhost:1234"

# Check results
curl http://localhost:5555/v2/_catalog

# Stop services
make down
```

See [.github/PR-TESTING.md](.github/PR-TESTING.md) for how PR validation works
in CI.

## Common Tasks

### Adding an Image

Create a GitHub issue with the `new-image` label, or manually add an entry to
`copa-config.yaml` under `images:` and create a PR.

## Troubleshooting

### "charts": null in catalog.json

Ensure all slice returns use empty slices (`[]Type{}`), not `nil`.

### Empty charts on site

Charts need reports embedded. Trigger the orchestrator workflow manually.

### OCI authentication issues

GHCR authentication uses `GITHUB_TOKEN` automatically in workflows.

## Getting Help

- **Issues**: https://github.com/verity-org/verity/issues
- **Discussions**: https://github.com/verity-org/verity/discussions
- **Security**: Report security issues privately through GitHub Security Advisories
