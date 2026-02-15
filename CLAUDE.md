# CLAUDE.md

Guide for AI assistants working on the Verity codebase.

## Project Overview

Verity is a self-maintaining registry of security-patched Helm charts. It scans Helm chart
dependencies for container image vulnerabilities, patches them using Microsoft Copa, and publishes
wrapper charts with patched images to OCI registries (Quay.io). The core application is written in
Go; the documentation site uses Astro.

## Repository Layout

```text
main.go              CLI entry point — flag parsing, mode dispatch
internal/            Core Go packages (single flat package)
  scanner.go         Image discovery in Helm charts and values files
  patcher.go         Copa patching pipeline, Trivy scanning
  matrix.go          GitHub Actions matrix generation from discovery manifests
  values.go          Helm values parsing and wrapper chart value generation
  helm.go            Chart downloading and dependency management
  sitedata.go        Site catalog JSON generation from reports
  *_test.go          Co-located unit tests for each module
site/                Astro-based static documentation site (Tailwind CSS)
charts/              Generated wrapper Helm charts (do not edit manually)
.github/workflows/   GitHub Actions CI/CD
  scan-and-patch.yaml  Main 3-job pipeline (discover → patch → assemble)
  ci.yaml              PR validation (unit tests, build, scan dry-run)
  lint.yaml            All linters (Go, YAML, shell, markdown, prettier)
  new-issue.yaml       Automated chart/image additions from issues
.github/scripts/     Workflow helper shell scripts
Chart.yaml           Helm chart dependencies to scan
values.yaml          Image overrides and standalone image definitions
```

## CLI Modes

The binary uses mutually exclusive mode flags:

- `-discover` — Scan charts, output GitHub Actions matrix JSON
- `-patch-single` — Patch a single image (used in matrix jobs)
- `-assemble` — Create wrapper charts from matrix results
- `-scan` — List discovered images without patching (dry run)
- `-site-data` — Generate site catalog JSON
- `-push-standalone-reports` — Push vulnerability reports to OCI

## Build & Test Commands

```bash
make build            # Compile Go binary → ./verity
make test             # Run all unit tests (go test -v ./...)
make test-coverage    # Tests + HTML coverage report
make fmt              # Format Go code (gofmt + goimports)
make fmt-strict       # Strict formatting (gofumpt)
make vet              # Run go vet
make lint             # Run golangci-lint (5 min timeout)
make lint-fmt         # Check formatting against gofumpt
make lint-vuln        # govulncheck for known vulnerabilities
make sec              # gosec security scanner
make quality          # ALL checks: fmt, vet, lint, tests, security, YAML, shell, markdown, frontend
make clean            # Remove build artifacts
make install-tools    # Install all tools via mise
```

Frontend (from `site/` directory):

```bash
cd site && npm ci             # Install dependencies
cd site && npm run lint       # ESLint
cd site && npm run format:check  # Prettier check
```

## Testing Conventions

- Standard Go `testing` package only — no external test frameworks
- Table-driven tests for comprehensive case coverage
- Test files are co-located: `foo.go` → `foo_test.go`
- Integration tests gated behind `RUN_INTEGRATION_TESTS=1` env var
- Run a specific package: `go test -v ./internal`

## Code Style & Formatting

### Go

- **Formatter**: `gofumpt` (stricter than gofmt) — enforced in CI
- **Indentation**: Tabs (standard Go style)
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` with descriptive context
- **Logging**: `fmt.Printf()` for stdout, `fmt.Fprintf(os.Stderr, ...)` for warnings
- **Fatal errors**: `log.Fatalf()` in main.go, return errors from internal package functions
- All code lives in a single `internal` package — no sub-packages
- Use empty slices (`[]Type{}`) instead of `nil` when the value will be serialized to JSON

### YAML/JSON/Markdown

- 2-space indentation for YAML, JSON, and Markdown
- YAML line length limit: 120 characters
- Markdown line length limit: 120 characters

### Frontend (site/)

- 2-space indentation for Astro, TypeScript, JavaScript, CSS
- Prettier with print width 100, ES5 trailing commas
- ESLint with Astro and TypeScript plugins

## Key Patterns

### Error Handling

Internal package functions return errors; main.go calls `log.Fatalf()`:

```go
// internal package — return wrapped errors
func DoSomething(path string) error {
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Errorf("reading %s: %w", path, err)
    }
    // ...
}

// main.go — fatal on error
if err := internal.DoSomething(path); err != nil {
    log.Fatalf("Failed: %v", err)
}
```

### Image References

Images are structured as `{Registry, Repository, Tag, Path}`. The `Reference()` method produces
the canonical `registry/repository:tag` string. The `Path` field tracks the JSON/YAML path in
chart values for override generation.

### Serialization

All data exchange between pipeline stages uses JSON files. The `DiscoveryManifest`, `PatchResult`,
and `SiteData` structs define the contract between discover/patch/assemble modes.

## Commit Message Convention

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

## CI Pipeline

PRs trigger:

1. **ci.yaml** — Unit tests, build verification, image scan dry-run
2. **lint.yaml** — golangci-lint, gofumpt, govulncheck, actionlint, yamllint, shellcheck,
   markdownlint, prettier

The main pipeline (`scan-and-patch.yaml`) runs on schedule and merge:

1. **Discover** — Parse Chart.yaml, download charts, scan for images, output matrix
2. **Patch** — One parallel job per image: pull → Trivy scan → Copa patch → push
3. **Assemble** — Collect results, build wrapper charts, generate site data

## Tool Versions

All tool versions are pinned in `mise.toml`. Install everything with `mise install`. Key versions:

- Go 1.25.7
- Node 22.22.0
- golangci-lint 1.64.8
- gofumpt 0.9.2

## Things to Watch Out For

- The `charts/` directory is auto-generated — never edit it by hand
- `values.yaml` at the root is project configuration, not a Helm values file for deployment
- The `internal` package is flat (no sub-packages); all types are in one namespace
- Pre-commit hooks enforce formatting — run `make fmt-strict` before committing if hooks fail
- CI skips linting on markdown-only and charts-only changes (see `paths-ignore` in ci.yaml)
