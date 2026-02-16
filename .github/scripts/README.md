# Workflow Scripts

Reusable scripts for GitHub Actions workflows. All scripts follow bash best practices with proper error handling.

## Installation Scripts

### install-copa.sh

Installs the latest version of Copa (Project Copacetic) from GitHub releases.

```bash
./.github/scripts/install-copa.sh
```

**Used by:** ci.yaml, scheduled-scan.yaml, patch-on-pr.yaml

**Note:** Trivy installation now uses the official `aquasecurity/setup-trivy` GitHub Action instead of a custom script.

---

## BuildKit Setup

Workflows use the **official** `docker/setup-buildx-action@v3` instead of a custom script.

This action:

- Sets up Docker Buildx (which uses BuildKit)
- Doesn't require privileged mode
- Creates a builder with an auto-generated name (accessible via action output)
- Automatically maintained by Docker

**Example workflow usage:**

```yaml
- name: Set up Docker Buildx
  id: buildx
  uses: docker/setup-buildx-action@v3

- name: Run Copa patching
  run: |
    ./verity-org -chart Chart.yaml -output charts \
      -patch \
      -buildkit-addr docker-container://${{ steps.buildx.outputs.name }}
```

---

## Validation Scripts

### validate-charts.sh

Validates wrapper chart structure, dependencies, and runs `helm lint`.

```bash
./.github/scripts/validate-charts.sh [directory]
```

**Arguments:**

- `directory` - Root directory containing charts/ folder (default: .)

**Checks:**

- Required files exist (Chart.yaml, values.yaml, .helmignore)
- Chart.yaml has valid apiVersion (v2)
- Exactly one dependency configured
- Helm lint passes

**Used by:** ci.yaml

---

## Publishing Scripts

### publish-charts.sh

Publishes wrapper charts to OCI registry.

```bash
./.github/scripts/publish-charts.sh <charts-dir> <registry> <org>
```

**Arguments:**

- `charts-dir` - Directory containing charts/ folder
- `registry` - OCI registry (e.g., ghcr.io)
- `org` - Organization name

**Example:**

```bash
./.github/scripts/publish-charts.sh . ghcr.io verity-org
```

**Actions:**

1. Builds Helm dependencies
2. Packages charts
3. Pushes to OCI registry

**Used by:** publish.yaml

---

### verify-images.sh

Verifies patched images exist in the registry.

```bash
./.github/scripts/verify-images.sh <charts-dir> <registry> <org>
```

**Arguments:**

- `charts-dir` - Directory containing charts/ folder
- `registry` - Docker registry (e.g., ghcr.io)
- `org` - Organization name

**Example:**

```bash
./.github/scripts/verify-images.sh . ghcr.io verity-org
```

**Actions:**

- Extracts image references from wrapper chart values
- Checks if each image exists using `docker manifest inspect`
- Reports missing images

**Used by:** publish.yaml

---

### generate-index.sh

Generates a markdown index of published charts.

```bash
./.github/scripts/generate-index.sh <charts-dir> <output-file> <registry> <org>
```

**Arguments:**

- `charts-dir` - Directory containing charts/ folder
- `output-file` - Path to output markdown file
- `registry` - OCI registry (e.g., ghcr.io)
- `org` - Organization name

**Example:**

```bash
./.github/scripts/generate-index.sh . /tmp/index.md ghcr.io verity-org
```

**Output:** Markdown file with installation commands for each chart

**Used by:** publish.yaml

---

## Git Scripts

### commit-changes.sh

Commits and pushes changes to a directory.

```bash
./.github/scripts/commit-changes.sh [directory]
```

**Arguments:**

- `directory` - Directory to commit (default: charts)

**Actions:**

1. Configures git user as github-actions[bot]
2. Checks for changes
3. Commits with standardized message
4. Pushes to current branch

**Used by:** patch-on-pr.yaml

---

## Best Practices

All scripts follow these conventions:

### Error Handling

```bash
set -euo pipefail
```

- `-e` - Exit on error
- `-u` - Error on undefined variables
- `-o pipefail` - Fail on pipeline errors

### Argument Validation

Scripts validate required arguments and show usage on error:

```bash
if [ -z "$ORG" ]; then
  echo "Usage: $0 <charts-dir> <registry> <org>"
  exit 1
fi
```

### Clear Output

- ✅ Success messages with checkmarks
- ❌ Error messages with X marks
- ⚠️ Warnings for non-critical issues
- Progress indicators for multi-step operations

### Idempotency

Scripts handle re-runs gracefully:

- Check if work already done
- Skip unnecessary steps
- Don't fail if nothing to do

---

## Testing Scripts Locally

### Prerequisites

```bash
# Install required tools
brew install yq jq helm docker
```

### Test Individual Scripts

```bash
# Validate syntax
bash -n .github/scripts/validate-charts.sh

# Dry run (most scripts support this)
./.github/scripts/validate-charts.sh /tmp/test-charts
```

### Test Full Workflow

```bash
# Start BuildKit
./.github/scripts/start-buildkit.sh

# Run verity-org with patching
./verity-org -chart Chart.yaml -output /tmp/test-charts \
  -patch \
  -buildkit-addr docker-container://buildkitd

# Validate output
./.github/scripts/validate-charts.sh /tmp/test-charts

# Cleanup
docker stop buildkitd
```

---

## Maintenance

### Updating Tool Versions

**BuildKit:**
Edit `start-buildkit.sh` default version or pass as argument.

**Trivy/Copa:**
Scripts automatically fetch latest release. No updates needed.

### Adding New Scripts

1. Create script in `.github/scripts/`
2. Set executable: `chmod +x .github/scripts/new-script.sh`
3. Add to this README
4. Update relevant workflows
5. Test locally before committing

### Script Template

```bash
#!/bin/bash
set -euo pipefail

# Script description and usage
if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <required-arg> [optional-arg]"
  exit 1
fi

REQUIRED="${1}"
OPTIONAL="${2:-default}"

echo "Starting operation..."

# Script logic here

echo "✅ Operation complete"
```
