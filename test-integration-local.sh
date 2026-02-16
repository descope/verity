#!/bin/bash
set -e

echo "=== Local Integration Test for Chart Assembly ==="
echo "(Tests chart creation, SBOM, and vuln aggregation without OCI push)"
echo ""

# Configuration
REGISTRY="localhost:5001"
TEST_DIR=".verity-test-local"

# Cleanup function
cleanup() {
  echo ""
  echo "=== Cleanup ==="
  rm -rf "$TEST_DIR"
}

trap cleanup EXIT

# Build verity
echo "1. Building verity..."
go build -o verity .
echo "   ✓ Build complete"
echo ""

# Clean test directory
rm -rf "$TEST_DIR"
mkdir -p "$TEST_DIR/results" "$TEST_DIR/reports"

# Create a test manifest with multiple charts
echo "2. Creating test manifest with multiple charts..."
cat > "$TEST_DIR/manifest.json" <<'EOF'
{
  "charts": [
    {
      "name": "test-app-changed",
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
    },
    {
      "name": "test-app-unchanged",
      "version": "2.0.0",
      "repository": "oci://example.com/charts",
      "images": [
        {
          "registry": "docker.io",
          "repository": "library/postgres",
          "tag": "16",
          "path": "database.image"
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
    },
    {
      "registry": "docker.io",
      "repository": "library/postgres",
      "tag": "16",
      "path": "database.image"
    }
  ]
}
EOF
echo "   ✓ Created manifest with 2 charts (1 changed, 1 unchanged)"
echo ""

# Create mock patch results
echo "3. Creating mock patch results..."

# Nginx - CHANGED (patched)
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

# Redis - CHANGED (newly mirrored, no fixable vulns)
cat > "$TEST_DIR/results/docker.io_library_redis_7.0.json" <<EOF
{
  "image_ref": "docker.io/library/redis:7.0",
  "patched_registry": "$REGISTRY",
  "patched_repository": "library/redis",
  "patched_tag": "7.0-patched",
  "vuln_count": 0,
  "skipped": true,
  "skip_reason": "no fixable vulnerabilities",
  "changed": true
}
EOF

# Postgres - UNCHANGED (already up to date)
cat > "$TEST_DIR/results/docker.io_library_postgres_16.json" <<EOF
{
  "image_ref": "docker.io/library/postgres:16",
  "patched_registry": "$REGISTRY",
  "patched_repository": "library/postgres",
  "patched_tag": "16-patched",
  "vuln_count": 0,
  "skipped": true,
  "skip_reason": "patched image up to date",
  "changed": false
}
EOF

echo "   ✓ nginx: changed=true (patched)"
echo "   ✓ redis: changed=true (newly mirrored)"
echo "   ✓ postgres: changed=false (already up to date)"
echo ""

# Create mock Trivy reports
echo "4. Creating mock Trivy reports..."

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
          "Title": "High severity vulnerability in nginx",
          "Description": "This is a mock high severity vulnerability"
        },
        {
          "VulnerabilityID": "CVE-2024-0002",
          "PkgName": "openssl",
          "Severity": "MEDIUM",
          "InstalledVersion": "1.1.1",
          "FixedVersion": "1.1.2",
          "Title": "Medium severity vulnerability in openssl",
          "Description": "This is a mock medium severity vulnerability"
        },
        {
          "VulnerabilityID": "CVE-2024-0003",
          "PkgName": "libssl",
          "Severity": "LOW",
          "InstalledVersion": "2.0.0",
          "FixedVersion": "2.0.1",
          "Title": "Low severity vulnerability",
          "Description": "This is a mock low severity vulnerability"
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

cat > "$TEST_DIR/reports/docker.io_library_postgres_16.json" <<'EOF'
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

# Run assemble WITHOUT --publish (just creates charts locally)
echo "5. Running assemble (without --publish)..."
echo ""
./verity assemble \
  --manifest "$TEST_DIR/manifest.json" \
  --results-dir "$TEST_DIR/results" \
  --reports-dir "$TEST_DIR/reports" \
  --output-dir "$TEST_DIR/charts" \
  --registry "$REGISTRY"
echo ""

# Verify results
echo "6. Verifying change detection..."

# Check that unchanged chart was skipped
if [ -d "$TEST_DIR/charts/test-app-unchanged" ]; then
  echo "   ✗ FAIL: test-app-unchanged should have been skipped (no changes)"
  exit 1
else
  echo "   ✓ test-app-unchanged was correctly skipped (no changes)"
fi

# Check that changed chart was created
if [ ! -d "$TEST_DIR/charts/test-app-changed" ]; then
  echo "   ✗ FAIL: test-app-changed should have been created"
  exit 1
else
  echo "   ✓ test-app-changed was correctly created (had changes)"
fi
echo ""

# Verify published-charts.json
echo "7. Verifying published-charts.json..."

if [ ! -f "$TEST_DIR/charts/published-charts.json" ]; then
  echo "   ✗ FAIL: published-charts.json not created"
  exit 1
fi

published_count=$(jq length "$TEST_DIR/charts/published-charts.json")
if [ "$published_count" -ne 1 ]; then
  echo "   ✗ FAIL: Expected 1 published chart, got $published_count"
  exit 1
fi

chart_name=$(jq -r '.[0].name' "$TEST_DIR/charts/published-charts.json")
if [ "$chart_name" != "test-app-changed" ]; then
  echo "   ✗ FAIL: Expected test-app-changed, got $chart_name"
  exit 1
fi

echo "   ✓ published-charts.json correct (1 chart)"
echo ""

# Verify chart details
echo "8. Verifying chart artifacts for test-app-changed..."

# Chart.yaml
if [ ! -f "$TEST_DIR/charts/test-app-changed/Chart.yaml" ]; then
  echo "   ✗ FAIL: Chart.yaml missing"
  exit 1
fi
version=$(grep '^version:' "$TEST_DIR/charts/test-app-changed/Chart.yaml" | awk '{print $2}')
echo "   ✓ Chart.yaml created (version: $version)"

# values.yaml
if [ ! -f "$TEST_DIR/charts/test-app-changed/values.yaml" ]; then
  echo "   ✗ FAIL: values.yaml missing"
  exit 1
fi
echo "   ✓ values.yaml created"

# SBOM
if [ ! -f "$TEST_DIR/charts/test-app-changed/sbom.cdx.json" ]; then
  echo "   ✗ FAIL: SBOM missing"
  exit 1
fi
sbom_format=$(jq -r '.bomFormat' "$TEST_DIR/charts/test-app-changed/sbom.cdx.json")
component_count=$(jq '.components | length' "$TEST_DIR/charts/test-app-changed/sbom.cdx.json")
if [ "$sbom_format" != "CycloneDX" ]; then
  echo "   ✗ FAIL: Invalid SBOM format"
  exit 1
fi
echo "   ✓ SBOM generated (CycloneDX, $component_count components)"

# Vuln predicate
if [ ! -f "$TEST_DIR/charts/test-app-changed/vuln-predicate.json" ]; then
  echo "   ✗ FAIL: Vulnerability predicate missing"
  exit 1
fi
vuln_count=$(jq '.vulnerabilities | length' "$TEST_DIR/charts/test-app-changed/vuln-predicate.json")
echo "   ✓ Vulnerability predicate generated ($vuln_count vulnerabilities)"
echo ""

# Inspect SBOM details
echo "9. Inspecting SBOM structure..."
echo "   Top-level component:"
jq -r '.metadata.component | "     \(.type): \(.name) v\(.version)"' "$TEST_DIR/charts/test-app-changed/sbom.cdx.json"
echo ""
echo "   Components:"
jq -r '.components[] | "     \(.type): \(.name) (\(.version // "no version"))"' "$TEST_DIR/charts/test-app-changed/sbom.cdx.json"
echo ""

# Inspect vulnerability predicate
echo "10. Inspecting vulnerability predicate..."
echo "   Scanner: $(jq -r '.scanner.uri' "$TEST_DIR/charts/test-app-changed/vuln-predicate.json")"
echo "   Vulnerabilities by severity:"
jq -r '.vulnerabilities | group_by(.Severity) | .[] | "     \(.[0].Severity): \(length) vuln(s)"' "$TEST_DIR/charts/test-app-changed/vuln-predicate.json"
echo ""

# Show published chart metadata
echo "11. Published chart metadata:"
jq '.[0] | {name, version, oci_ref, image_count: (.images | length)}' "$TEST_DIR/charts/published-charts.json"
echo ""

# Summary
echo "=== Integration Test PASSED ✓ ==="
echo ""
echo "Summary:"
echo "  • Change detection: WORKING"
echo "  • Chart creation: WORKING"
echo "  • SBOM generation: WORKING"
echo "  • Vulnerability aggregation: WORKING"
echo "  • published-charts.json: WORKING"
echo ""
echo "Test artifacts available in: $TEST_DIR/charts/"
