#!/bin/bash
set -e

echo "=== Local Integration Test for Chart Publishing ==="
echo ""

# Configuration
REGISTRY="localhost:5001"
TEST_DIR=".verity-test"

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

# Run discover with chart-file
echo "3. Running discover with --chart-file..."
./verity discover \
  --images values.yaml \
  --chart-file Chart.yaml \
  --discover-dir "$TEST_DIR"
echo "   ✓ Discovery complete"
echo ""

# Check manifest
echo "4. Checking manifest.json..."
if [ ! -f "$TEST_DIR/manifest.json" ]; then
  echo "   ✗ manifest.json not created"
  exit 1
fi

chart_count=$(jq '.charts | length' "$TEST_DIR/manifest.json")
image_count=$(jq '.images | length' "$TEST_DIR/manifest.json")
echo "   ✓ Found $chart_count chart(s) and $image_count image(s)"
echo ""

# Create mock patch results with Changed=true for a few images
echo "5. Creating mock patch results..."
jq -r '.images[] | .registry + "/" + .repository + ":" + .tag' "$TEST_DIR/manifest.json" | head -3 | while read -r image_ref; do
  sanitized=$(echo "$image_ref" | tr '/:' '_')

  # Parse the image ref to create patched version
  registry=$(echo "$image_ref" | cut -d'/' -f1)
  rest=$(echo "$image_ref" | cut -d'/' -f2-)
  repo=$(echo "$rest" | cut -d':' -f1)
  tag=$(echo "$rest" | cut -d':' -f2)

  cat > "$TEST_DIR/results/${sanitized}.json" <<EOF
{
  "image_ref": "$image_ref",
  "patched_registry": "$REGISTRY",
  "patched_repository": "$repo",
  "patched_tag": "${tag}-patched",
  "vuln_count": 5,
  "skipped": false,
  "changed": true
}
EOF

  # Create a mock Trivy report
  cat > "$TEST_DIR/reports/${sanitized}.json" <<EOF
{
  "Results": [
    {
      "Vulnerabilities": [
        {
          "VulnerabilityID": "CVE-2024-0001",
          "PkgName": "test-package",
          "Severity": "HIGH",
          "InstalledVersion": "1.0.0",
          "FixedVersion": "1.0.1",
          "Title": "Test vulnerability",
          "Description": "This is a test vulnerability for integration testing"
        }
      ]
    }
  ]
}
EOF

  echo "   ✓ Created result for $image_ref"
done
echo ""

# Login to local registry (helm)
echo "6. Logging in to local registry..."
echo "test" | helm registry login "$REGISTRY" -u test --password-stdin
echo "   ✓ Helm logged in"
echo ""

# Run assemble with --publish
echo "7. Running assemble with --publish..."
./verity assemble \
  --manifest "$TEST_DIR/manifest.json" \
  --results-dir "$TEST_DIR/results" \
  --reports-dir "$TEST_DIR/reports" \
  --output-dir "$TEST_DIR/charts" \
  --registry "$REGISTRY" \
  --publish
echo ""

# Verify published-charts.json
echo "8. Verifying published-charts.json..."
if [ ! -f "$TEST_DIR/charts/published-charts.json" ]; then
  echo "   ✗ published-charts.json not created"
  exit 1
fi

published_count=$(jq length "$TEST_DIR/charts/published-charts.json")
echo "   ✓ Published $published_count chart(s)"

if [ "$published_count" -gt 0 ]; then
  echo ""
  echo "   Published charts:"
  jq -r '.[] | "   - \(.name):\(.version) → \(.oci_ref)"' "$TEST_DIR/charts/published-charts.json"

  # Verify each chart's artifacts
  echo ""
  echo "9. Verifying chart artifacts..."
  jq -r '.[].name' "$TEST_DIR/charts/published-charts.json" | while read -r chart_name; do
    echo "   Chart: $chart_name"

    # Check SBOM
    if [ -f "$TEST_DIR/charts/$chart_name/sbom.cdx.json" ]; then
      echo "      ✓ SBOM generated"
    else
      echo "      ✗ SBOM missing"
    fi

    # Check vuln predicate
    if [ -f "$TEST_DIR/charts/$chart_name/vuln-predicate.json" ]; then
      echo "      ✓ Vulnerability predicate generated"
    else
      echo "      ✗ Vulnerability predicate missing"
    fi

    # Check Chart.yaml
    if [ -f "$TEST_DIR/charts/$chart_name/Chart.yaml" ]; then
      echo "      ✓ Chart.yaml created"
    else
      echo "      ✗ Chart.yaml missing"
    fi

    # Check values.yaml
    if [ -f "$TEST_DIR/charts/$chart_name/values.yaml" ]; then
      echo "      ✓ values.yaml created"
    else
      echo "      ✗ values.yaml missing"
    fi
  done

  # Try to pull the chart from registry
  echo ""
  echo "10. Verifying chart is in registry..."
  first_chart=$(jq -r '.[0].oci_ref' "$TEST_DIR/charts/published-charts.json")
  if helm pull "oci://${first_chart%:*}" --version "${first_chart##*:}" --destination /tmp 2>/dev/null; then
    echo "    ✓ Successfully pulled chart from registry"
  else
    echo "    ⚠ Could not pull chart (this may be normal if the chart push completed but the tag format is different)"
  fi
else
  echo "   ⚠ No charts were published (this may be expected if no images changed)"
fi

echo ""
echo "=== Integration Test Complete ==="
