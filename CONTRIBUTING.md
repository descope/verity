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
make quality

# Or run individual checks:
make fmt           # Format code
make lint          # Run golangci-lint
make vet           # Run go vet
make sec           # Run security scanner
make test          # Run tests
make lint-workflows  # Lint GitHub Actions
make lint-yaml     # Lint YAML files
make lint-shell    # Lint shell scripts
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

- **patch-matrix.yaml**: Main pipeline (scan → patch → sign → publish)
  - Triggers: PR validation, push to main, daily scheduled scans
- **ci.yaml**: Unit tests on pull requests
- **lint.yaml**: Code quality checks (8 linters)
- **new-issue.yaml**: Automated image additions via GitHub issues

### Key Components

- **cmd/**: CLI commands
  - `scan.go`: Parallel Trivy scanning
  - `catalog.go`: Site catalog generation
- **internal/**: Core logic
  - `copaconfig.go`: `copa-config.yaml` parsing and image discovery
  - `sitedata.go`: Catalog JSON generation from Trivy reports
  - `types.go`: Image reference models and parsing
- **.github/scripts/**: Workflow helper scripts
- **site/**: Astro-based static site

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

Charts need reports embedded. Trigger the patch-matrix workflow manually.

### OCI authentication issues

GHCR authentication uses `GITHUB_TOKEN` automatically in workflows.

## Getting Help

- **Issues**: https://github.com/verity-org/verity/issues
- **Discussions**: https://github.com/verity-org/verity/discussions
- **Security**: Report security issues privately through GitHub Security Advisories
