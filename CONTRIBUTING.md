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
git clone https://github.com/descope/verity.git
cd verity

# Install ALL tools via mise (recommended)
mise install
# Installs: go, node, golangci-lint, gofumpt, govulncheck,
#           gosec, goimports, actionlint, shellcheck, yamllint,
#           markdownlint, helm, crane, claude-code

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

### Workflows

- **scan-and-patch.yaml**: Main workflow for scanning, patching, and publishing
  - Triggers: PR validation, push to main, scheduled scans
- **publish.yaml**: Site deployment only
- **new-issue.yaml**: Automated chart/image additions via GitHub issues

### Key Components

- **cmd/**: CLI entry point
- **internal/**: Core logic
  - `patcher.go`: Image patching with Copa
  - `sitedata.go`: Site catalog generation
  - `discover.go`: Image discovery
- **.github/scripts/**: Workflow helper scripts
- **site/**: Astro-based static site

## Common Tasks

### Adding a New Chart

Create a GitHub issue with the `new-chart` label, or manually:

```bash
# Add to Chart.yaml dependencies
# Add to values.yaml with patched image config
# Create PR
```

### Adding an Image

Create a GitHub issue with the `new-image` label, or manually add it to
`values.yaml` and create a PR. The discover step deduplicates by image
reference, so images shared across charts only appear once.

### Updating Chart Versions

Renovate handles this automatically. For manual updates:

```bash
# Update Chart.yaml dependency version
# Create PR → scan-and-patch validates → merge → publishes
```

## Troubleshooting

### "charts": null in catalog.json

Ensure all slice returns use empty slices (`[]Type{}`), not `nil`.

### Empty charts on site

Charts need reports embedded. Trigger scan-and-patch workflow manually.

### OCI authentication issues

Ensure `QUAY_USERNAME` and `QUAY_PASSWORD` secrets are set.

## Getting Help

- **Issues**: https://github.com/descope/verity/issues
- **Discussions**: https://github.com/descope/verity/discussions
- **Security**: Report security issues privately to security@descope.com
