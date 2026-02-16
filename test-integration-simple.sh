#!/bin/bash
set -e

echo "=== Simple Local Integration Test for Chart Publishing ==="
echo ""

# Configuration
REGISTRY="localhost:5001"
TEST_DIR=".verity-test-simple"

# Cleanup function
cleanup() {
  echo ""
  echo "=== Cleanup ==="
  docker stop test-registry 2>/dev/null || true
  docker rm test-registry 2>/dev/null || true
  rm -rf "$TEST_DIR"
}

trap cleanup EXIT

# Start local Docker registry
echo "1. Starting local Docker registry at $REGISTRY..."
docker run -d -p 5001:5000 --name test-registry registry:2
sleep 2
echo "   ✓ Registry running"
echo ""

# Build verity
echo "2. Building verity..."
go build -o verity .
echo "   ✓ Build complete"
echo ""

# Clean test directory
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR/results" "$TEST_DIR/reports"

# Create a minimal manifest manually (skipping discover step)
echo "3. Creating test manifest..."
cat > "$TEST_DIR/manifest.json" <<'EOF'
{
  "charts": [
    {
      "name": "test-app",
      "version": "1.0.0",
      "repository": "oci://example.com/charts",
      "images": [
        {
          "registry": "docker.io",
          "repository": "library/nginx",
          "tag": "1.25",
          "path": "image"
        },
        {
          "registry": "docker.io",
          "repository": "library/redis",
          "tag": "7.0",
          "path": "redis.image"
        }
      ]
    }
  ],
  "images": [
    {
      "registry": "docker.io",
      "repository": "library/nginx",
      "tag": "1.25",
      "path": "image"
    },
    {
      "registry": "docker.io",
      "repository": "library/redis",
      "tag": "7.0",
      "path": "redis.image"
    }
  ]
}
EOF
echo "   ✓ Test manifest created"
echo ""

# Create mock patch results with Changed=true
echo "4. Creating mock patch results..."

# Nginx result - changed
cat > "$TEST_DIR/results/docker.io_library_nginx_1.25.json" <<EOF
{
  "image_ref": "docker.io/library/nginx:1.25",
  "patched_registry": "$REGISTRY",
  "patched_repository": "library/nginx",
  "patched_tag": "1.25-patched",
  "vuln_count": 3,
  "skipped": false,
  "changed": true
}
EOF

# Redis result - unchanged (to test mixed scenario)
cat > "$TEST_DIR/results/docker.io_library_redis_7.0.json" <<EOF
{
  "image_ref": "docker.io/library/redis:7.0",
  "patched_registry": "$REGISTRY",
  "patched_repository": "library/redis",
  "patched_tag": "7.0-patched",
  "vuln_count": 0,
  "skipped": true,
  "skip_reason": "patched image up to date",
  "changed": false
}
EOF

echo "   ✓ Created nginx result (changed=true)"
echo "   ✓ Created redis result (changed=false)"
echo ""

# Create mock Trivy reports
echo "5. Creating mock Trivy reports..."

cat > "$TEST_DIR/reports/docker.io_library_nginx_1.25.json" <<'EOF'
{
  "Results": [
    {
      "Vulnerabilities": [
        {
          "VulnerabilityID": "CVE-2024-0001",
          "PkgName": "nginx",
          "Severity": "HIGH",
          "InstalledVersion": "1.25.0",
          "FixedVersion": "1.25.1",
          "Title": "Test vulnerability in nginx",
          "Description": "This is a mock vulnerability for testing"
        },
        {
          "VulnerabilityID": "CVE-2024-0002",
          "PkgName": "openssl",
          "Severity": "MEDIUM",
          "InstalledVersion": "1.1.1",
          "FixedVersion": "1.1.2",
          "Title": "Test vulnerability in openssl",
          "Description": "Another mock vulnerability"
        }
      ]
    }
  ]
}
EOF

cat > "$TEST_DIR/reports/docker.io_library_redis_7.0.json" <<'EOF'
{
  "Results": [
    {
      "Vulnerabilities": []
    }
  ]
}
EOF

echo "   ✓ Created Trivy reports"
echo ""

# Login to local registry (helm)
echo "6. Logging in to local registry..."
echo "test" | helm registry login "$REGISTRY" -u test --password-stdin 2>/dev/null
echo "   ✓ Helm logged in"
echo ""

# Run assemble with --publish
echo "7. Running assemble with --publish..."
echo ""
./verity assemble \
  --manifest "$TEST_DIR/manifest.json" \
  --results-dir "$TEST_DIR/results" \
  --reports-dir "$TEST_DIR/reports" \
  --output-dir "$TEST_DIR/charts" \
  --registry "$REGISTRY" \
  --publish
echo ""

# Verify results
echo "8. Verifying results..."
echo ""

if [ ! -f "$TEST_DIR/charts/published-charts.json" ]; then
  echo "   ✗ published-charts.json not created"
  exit 1
fi

published_count=$(jq length "$TEST_DIR/charts/published-charts.json")
echo "   ✓ Published $published_count chart(s)"

if [ "$published_count" -gt 0 ]; then
  echo ""
  echo "   Published charts:"
  jq -r '.[] | "     \(.name):\(.version) → \(.oci_ref)"' "$TEST_DIR/charts/published-charts.json"

  echo ""
  echo "9. Verifying chart artifacts..."
  jq -r '.[].name' "$TEST_DIR/charts/published-charts.json" | while read -r chart_name; do
    echo "   Chart: $chart_name"

    # Check Chart.yaml
    if [ -f "$TEST_DIR/charts/$chart_name/Chart.yaml" ]; then
      version=$(grep '^version:' "$TEST_DIR/charts/$chart_name/Chart.yaml" | awk '{print $2}')
      echo "      ✓ Chart.yaml created (version: $version)"
    else
      echo "      ✗ Chart.yaml missing"
    fi

    # Check values.yaml
    if [ -f "$TEST_DIR/charts/$chart_name/values.yaml" ]; then
      echo "      ✓ values.yaml created"
    else
      echo "      ✗ values.yaml missing"
    fi

    # Check SBOM
    if [ -f "$TEST_DIR/charts/$chart_name/sbom.cdx.json" ]; then
      component_count=$(jq '.components | length' "$TEST_DIR/charts/$chart_name/sbom.cdx.json")
      echo "      ✓ SBOM generated ($component_count components)"
    else
      echo "      ✗ SBOM missing"
    fi

    # Check vuln predicate
    if [ -f "$TEST_DIR/charts/$chart_name/vuln-predicate.json" ]; then
      vuln_count=$(jq '.vulnerabilities | length' "$TEST_DIR/charts/$chart_name/vuln-predicate.json")
      echo "      ✓ Vulnerability predicate generated ($vuln_count vulns)"
    else
      echo "      ✗ Vulnerability predicate missing"
    fi
  done

  echo ""
  echo "10. Inspecting generated SBOM..."
  first_chart=$(jq -r '.[0].name' "$TEST_DIR/charts/published-charts.json")
  if [ -f "$TEST_DIR/charts/$first_chart/sbom.cdx.json" ]; then
    echo "    Components:"
    jq -r '.components[] | "      - \(.type): \(.name) (\(.version // "no version"))"' "$TEST_DIR/charts/$first_chart/sbom.cdx.json"
  fi

  echo ""
  echo "11. Checking published chart metadata..."
  jq '.[0]' "$TEST_DIR/charts/published-charts.json"
else
  echo "   ⚠ No charts were published (expected at least 1)"
  exit 1
fi

echo ""
echo "=== Integration Test PASSED ==="
