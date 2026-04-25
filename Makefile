.PHONY: help build test test-coverage lint lint-vuln lint-workflows lint-yaml lint-shell lint-markdown lint-frontend fmt-frontend check-frontend integer-validate integer-gen integer-build-all integer-melange-prep chart-integration clean install-tools quality up down

# Default target
help:
	@echo "Available targets:"
	@echo "  make build            - Build the verity binary"
	@echo "  make test             - Run unit tests"
	@echo "  make up               - Start local registry + BuildKit"
	@echo "  make down             - Stop local test environment"
	@echo "  make lint             - Run Go linter (golangci-lint with gofumpt, goimports, gosec)"
	@echo "  make quality          - Run ALL linters and tests"
	@echo "  make chart-integration - Smoke-test published wrapper charts on a kind cluster"
	@echo "  make clean            - Clean build artifacts"
	@echo "  make install-tools    - Install development tools via mise"

# Build binary
build:
	go build -o verity .

# Run tests
test:
	go test -v ./...

# Run tests with coverage
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run golangci-lint (includes gofumpt, goimports, gosec, and more)
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Run: make install-tools" && exit 1)
	golangci-lint run --timeout=5m

# Fix Go formatting and imports (via golangci-lint)
fmt:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Run: make install-tools" && exit 1)
	golangci-lint run --fix --timeout=5m

# Check for Go vulnerabilities
lint-vuln:
	@which govulncheck > /dev/null || (echo "govulncheck not found. Run: make install-tools" && exit 1)
	govulncheck ./...

# Lint workflows
lint-workflows:
	@which actionlint > /dev/null || (echo "actionlint not found. Run: make install-tools" && exit 1)
	actionlint

# Lint YAML files
lint-yaml:
	@which yamllint > /dev/null || (echo "yamllint not found. Run: make install-tools" && exit 1)
	yamllint -c .yamllint.yml .

# Lint shell scripts
lint-shell:
	@which shellcheck > /dev/null || (echo "shellcheck not found. Run: make install-tools" && exit 1)
	shellcheck .github/scripts/*.sh scripts/*.sh

# Lint markdown files
lint-markdown:
	@which markdownlint > /dev/null || (echo "markdownlint not found. Run: make install-tools" && exit 1)
	markdownlint "*.md" "**/*.md" --ignore node_modules --ignore site/node_modules

# Lint and check frontend code
lint-frontend:
	cd site && npm run lint

# Format frontend code
fmt-frontend:
	cd site && npx prettier --write "src/**/*.{js,ts,astro,css,json,md}"

# Check frontend formatting
check-frontend:
	cd site && npx prettier --check "src/**/*.{js,ts,astro,css,json,md}"

# ── Integer (Image Configuration) Targets ────────────────────────────────

# Validate all image configs (schema + file existence)
integer-validate: build
	./verity integer validate

# Generate all apko configs into ./gen/ (fast — no apko required)
integer-gen: build
	./verity integer discover --gen-dir ./gen
	@echo "✓ Generated apko configs → gen/"

# Build all image variants locally with apko (amd64 only, no push)
# Runs apko for each of the 173+ variants — takes a long time.
# Narrow scope with: IMAGE=node make integer-build-all
integer-build-all: build
	@which apko > /dev/null || (echo "apko not found. Run: make install-tools" && exit 1)
	./verity integer discover --gen-dir ./gen | \
	  jq -r '.[] | [.name, .version, .type] | @tsv' | \
	  $(if $(IMAGE),grep "^$(IMAGE)	",cat) | \
	  while IFS=$$'\t' read -r name version type; do \
	    echo "Building $$name:$$version-$$type..."; \
	    apko build --arch amd64 \
	      "gen/$$name/$$version/$$type.apko.yaml" \
	      "$$name:$$version-$$type" \
	      /dev/null || exit 1; \
	  done
	@echo "✓ All images built"

# Run melange prep+build locally for a single image type (mirrors CI exactly).
# Usage: IMAGE=caddy TYPE=fips make integer-melange-prep
integer-melange-prep:
	@[ -n "$(IMAGE)" ] || (echo "Usage: IMAGE=caddy TYPE=fips make integer-melange-prep" && exit 1)
	@[ -n "$(TYPE)" ] || (echo "Usage: IMAGE=caddy TYPE=fips make integer-melange-prep" && exit 1)
	@echo "$(IMAGE)" | grep -Eq '^[a-z][a-z0-9-]*$$' || (echo "Invalid IMAGE: '$(IMAGE)'. Expected ^[a-z][a-z0-9-]*$$" && exit 1)
	@echo "$(TYPE)"  | grep -Eq '^[a-z][a-z0-9-]*$$' || (echo "Invalid TYPE: '$(TYPE)'. Expected ^[a-z][a-z0-9-]*$$"  && exit 1)
	@which melange > /dev/null || (echo "melange not found. Run: mise install" && exit 1)
	bash scripts/integer-melange-prep.sh "$(IMAGE)" "$(TYPE)"

# Run all quality checks (golangci-lint handles gofumpt, goimports, vet, gosec)
quality: lint lint-vuln lint-workflows lint-yaml lint-shell lint-markdown check-frontend integer-validate test
	@echo "✓ All quality checks passed!"

# Clean build artifacts
clean:
	rm -f verity
	rm -f coverage.out coverage.html
	rm -rf .verity/
	rm -rf site/dist/

# Install development tools
install-tools:
	@echo "Installing tools via mise..."
	@which mise > /dev/null || (echo "mise not found. Install from: https://mise.jdx.dev" && exit 1)
	mise install
	@echo ""
	@echo "✓ All tools installed via mise!"
	@echo ""
	@echo "Installed tools:"
	@echo "  - golangci-lint (Go linter)"
	@echo "  - actionlint (GitHub Actions linter)"
	@echo "  - yamllint (YAML linter)"
	@echo "  - shellcheck (Shell script linter)"
	@echo ""
	@echo "Run 'mise list' to see all installed tools"

# ── Local Testing with Docker ────────────────────────────────────────

# Start local test environment (registry + buildkit)
up:
	@echo "Starting local registry and BuildKit..."
	docker compose up -d
	@echo "Waiting for services to be ready..."
	@sleep 3
	@echo ""
	@echo "✓ Local registry:  localhost:5555"
	@echo "✓ BuildKit:        tcp://localhost:1234"

# Stop local test environment
down:
	docker compose down

# ── Chart Integration Smoke Tests ─────────────────────────────────────

# Smoke-test the wrapper charts published nightly to ghcr.io/verity-org/charts.
# Spins up a kind cluster, helm-installs each chart in Chart.yaml from the
# published OCI registry, waits for healthy, asserts every running container
# image is from ghcr.io/verity-org and no container restarted within 30s.
#
#   make chart-integration                           # all charts in Chart.yaml
#   VERITY_CHART=victoria-logs-single make chart-integration   # single chart
chart-integration:
	@which kind > /dev/null    || (echo "kind not found. Run: make install-tools" && exit 1)
	@which kubectl > /dev/null || (echo "kubectl not found. Run: make install-tools" && exit 1)
	@which helm > /dev/null    || (echo "helm not found. Run: make install-tools" && exit 1)
	@which docker > /dev/null  || (echo "docker not found in PATH" && exit 1)
	go test -tags=integration -v -timeout=45m ./test/chart-integration/...
