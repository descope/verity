.PHONY: help build test test-coverage lint lint-vuln lint-workflows lint-yaml lint-shell lint-markdown lint-frontend fmt-frontend check-frontend clean install-tools quality up down

# Default target
help:
	@echo "Available targets:"
	@echo "  make build            - Build the verity binary"
	@echo "  make test             - Run unit tests"
	@echo "  make up               - Start local registry + BuildKit"
	@echo "  make down             - Stop local test environment"
	@echo "  make lint             - Run Go linter (golangci-lint with gofumpt, goimports, gosec)"
	@echo "  make quality          - Run ALL linters and tests"
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
	shellcheck .github/scripts/*.sh

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

# Run all quality checks (golangci-lint handles gofumpt, goimports, vet, gosec)
quality: lint lint-vuln lint-workflows lint-yaml lint-shell lint-markdown check-frontend test
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
