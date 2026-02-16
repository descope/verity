.PHONY: help build test test-coverage lint lint-fmt lint-vuln lint-workflows lint-yaml lint-shell lint-markdown lint-frontend fmt fmt-strict fmt-frontend check-frontend vet sec clean install-tools quality test-local test-local-patch up down

# Default target
help:
	@echo "Available targets:"
	@echo "  make build            - Build the verity binary"
	@echo "  make test             - Run unit tests"
	@echo "  make test-local       - Run integration tests with local registry"
	@echo "  make test-local-patch - Test patching with local registry"
	@echo "  make up               - Start local registry + BuildKit"
	@echo "  make down             - Stop local test environment"
	@echo "  make lint             - Run Go linter (golangci-lint)"
	@echo "  make quality          - Run ALL linters and tests"
	@echo "  make fmt              - Format code"
	@echo "  make vet              - Run go vet"
	@echo "  make sec              - Run security scanner (gosec)"
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

# Format code
fmt:
	gofmt -s -w .
	goimports -w .

# Run go vet
vet:
	go vet ./...

# Run security scanner
sec:
	@which gosec > /dev/null || (echo "gosec not found. Run: make install-tools" && exit 1)
	gosec -quiet ./...

# Run golangci-lint
lint:
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Run: make install-tools" && exit 1)
	golangci-lint run --timeout=5m

# Check Go formatting (gofumpt)
lint-fmt:
	@which gofumpt > /dev/null || (echo "gofumpt not found. Run: make install-tools" && exit 1)
	@echo "Checking Go formatting..."
	@gofumpt -l . | tee /tmp/gofumpt.txt
	@if [ -s /tmp/gofumpt.txt ]; then \
		echo "Files need formatting. Run: make fmt-strict"; \
		exit 1; \
	fi

# Strict formatting (gofumpt)
fmt-strict:
	@which gofumpt > /dev/null || (echo "gofumpt not found. Run: make install-tools" && exit 1)
	gofumpt -w .

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

# Run all quality checks
quality: fmt vet lint lint-fmt lint-vuln lint-workflows lint-yaml lint-shell lint-markdown check-frontend sec test
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
	@echo ""
	@echo "Use 'make test-local' to run integration tests"

# Stop local test environment
down:
	docker compose down

# Run integration tests with local registry (fast - no patching)
test-local: build
	@echo "Running local integration tests..."
	@echo ""
	@echo "→ Testing discover..."
	./verity discover --discover-dir .verity
	@echo ""
	@echo "→ Testing list..."
	./verity list | head -10
	@echo ""
	@echo "✓ Integration tests complete"

# Test patching with local registry (requires: make up)
test-local-patch: build
	@echo "Testing patch with local registry..."
	@echo "Note: Make sure local registry is running (make up)"
	@echo ""
	./verity patch \
		--image "docker.io/library/nginx:1.29.5" \
		--registry "localhost:5555/verity" \
		--buildkit-addr "tcp://localhost:1234" \
		--result-dir .verity/results \
		--report-dir .verity/reports
	@echo ""
	@echo "✓ Patch test complete"
	@echo "Check .verity/results/ for patch results"
