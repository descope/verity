# Migration Complete ✅

## Summary

Successfully migrated Verity from Quay.io to GHCR and removed chart concept to focus purely on patched container images.

## Changes Made

### 1. Quay.io → GHCR Migration ✅

- **Target registry**: `quay.io/verity` → `ghcr.io/verity-org`
- **Authentication**: Removed `QUAY_USERNAME`, `QUAY_PASSWORD`, `QUAY_API_TOKEN` → Uses `GITHUB_TOKEN`
- **Visibility**: Removed `quay-make-public.sh` (GHCR is public by default)
- **Intentionally kept**: Upstream source registries in `values.yaml` (quay.io/prometheus, docker.io/grafana, etc.)

### 2. Removed Chart Concept ✅

**Deleted:**

- Chart.yaml
- charts/ directory (3 wrapper charts removed)
- .github/scripts/publish-charts.sh
- .github/scripts/sign-charts.sh  
- .github/scripts/check-chart-changes.sh
- .github/scripts/commit-changes.sh
- .github/scripts/add-chart-dependency.sh
- .github/scripts/validate-charts.sh
- .github/scripts/generate-index.sh
- quay-make-public.sh

**Simplified:**

- main.go: Removed `assemble` mode, removed `-chart` flag
- Workflow: Removed chart assembly, chart publishing, chart signing jobs

### 3. New Architecture ✅

```text
values.yaml → discover → patch (matrix) → sign/attest → ghcr.io/verity-org
```

**CLI Modes (4 total):**

- `discover` - Parse values.yaml, output matrix.json
- `patch-single` - Patch one image (matrix job)
- `list` - List images (dry run)
- `site-data` - Generate catalog JSON

**Workflow Jobs (5 total):**

1. Discover images
2. Patch (parallel matrix)
3. Generate catalog (optional)
4. Deploy site  
5. Upload reports

### 4. Testing ✅

- All tests pass
- CLI modes verified:
  - `./verity -list` → Lists 19 images
  - `./verity -discover` → Generates matrix with 19 images
  - `./verity -site-data` → Generates catalog (0 charts, 19 images)
- Build succeeds
- Workflow YAML is valid

### 5. Documentation Updates ✅

- site/src/pages/compliance.astro: Removed chart verification example, updated pipeline
- CONTRIBUTING.md: Updated OCI authentication section
- README.md: Already updated by user/linter

## Statistics

- **Files changed**: 37
- **Lines added**: 199
- **Lines removed**: 1,011
- **Net change**: -812 lines (81% reduction!)

## Remaining Scripts

These scripts are still present and functional:

- add-standalone-image.sh
- install-copa.sh
- parse-image-issue-form.sh
- parse-issue-form.sh
- verify-artifacts.sh
- verify-images.sh

## Registry References Explained

**values.yaml contains `quay.io` references - this is CORRECT:**

- These are **upstream source registries** we pull FROM
- Examples: `quay.io/prometheus/prometheus`, `quay.io/opstree/redis`
- We patch these and push to `ghcr.io/verity-org/*-patched`

**Our target registry:**

- `ghcr.io/verity-org/*-patched` (all patched images go here)

## Verification Commands

```bash
# Build
make build

# Test
make test

# List images
./verity -list -images values.yaml

# Discover
./verity -discover -images values.yaml -discover-dir .verity

# Generate site data
./verity -site-data /tmp/catalog.json -images values.yaml -registry ghcr.io/verity-org
```

All commands work successfully!

## What's Next

The project is now:

- ✅ Using GHCR with automatic GitHub authentication
- ✅ Focused purely on patched container images
- ✅ Simpler architecture (no charts)
- ✅ All tests passing
- ✅ Ready for use

**No additional work required - migration is complete!**
