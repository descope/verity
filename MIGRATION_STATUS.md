# Copa Bulk Config Mode Migration - Status

## ✅ Phase 2 Complete: New Config Files + Post-Process Command

### Files Created

1. **`copa-config.yaml`** - Copa bulk config with pattern-based tag strategies
   - 21 images configured across Docker Hub, GHCR, Quay.io, and registry.k8s.io
   - Pattern regex matches for version discovery
   - Targets Copa-compatible variants (e.g., `debian` for vector instead of `distroless`)

2. **`chart-image-map.yaml`** - Chart-to-image grouping for wrapper chart assembly
   - 4 charts: prometheus, victoria-logs-single, postgres-operator, standalone
   - Maps images to their source charts for downstream assembly
   - Preserves chart versions from Chart.yaml

3. **`internal/copaconfig.go`** - Types and parsers for Copa output and chart mapping
   - `ParseCopaOutput()` - reads Copa's `--output-json` file
   - `ParseChartImageMap()` - reads chart-image-map.yaml
   - `ParseImageRef()` - parses image references into components
   - `NormalizeImageRef()` - canonicalizes image references for comparison

4. **`internal/postprocess.go`** - Core post-processing logic
   - `PostProcessCopaResults()` - main orchestrator
   - Generates matrix.json for GitHub Actions attest job
   - Generates manifest.json for assemble step
   - Writes per-image SinglePatchResult files for compatibility
   - Queries registry for digests (with skip flag for testing)

5. **`cmd/postprocess.go`** - CLI command
   - `verity post-process` command with flags:
     - `--copa-output` - path to Copa's output JSON
     - `--chart-map` - path to chart-image-map.yaml
     - `--registry` - target registry prefix
     - `--output-dir` - output directory (default: .verity)
     - `--skip-digest-lookup` - skip registry queries (for testing)

6. **Test files**:
   - `internal/copaconfig_test.go` - comprehensive unit tests for parsing functions
   - `internal/postprocess_test.go` - end-to-end tests for post-processing logic

### Files Modified

1. **`main.go`**
   - Added `PostProcessCommand` to the command list
   - Kept legacy commands (scan, discover, patch) for Phase 4 removal

### Testing

✅ All tests pass:
- `go test ./...` - full test suite passes
- Unit tests for `ParseImageRef()`, `NormalizeImageRef()`, parsers
- Integration test for `PostProcessCopaResults()` with mock data
- Command line interface tested manually

### Verification

Command works end-to-end:
```bash
./verity post-process \
  --copa-output test-copa-output.json \
  --chart-map chart-image-map.yaml \
  --registry ghcr.io/verity-org \
  --output-dir .verity \
  --skip-digest-lookup
```

Produces:
- `.verity/matrix.json` - GitHub Actions matrix (compact JSON)
- `.verity/manifest.json` - Chart-grouped image manifest
- `.verity/results/*.json` - Per-image SinglePatchResult files

---

## 🔄 Next Steps: Phase 3 - Workflow Migration

### Required for Phase 3

1. **Wait for Copa PR #1475** to be merged with `--output-json` flag
   - Currently blocks CI integration
   - Local testing can continue with mock data

2. **Workflow rewrite**: `.github/workflows/scan-and-patch.yaml`
   - Job 1: Copa bulk patch (replaces scan + discover + patch matrix)
   - Job 2: Post-process (new - calls `verity post-process`)
   - Job 3: Attest images (existing, uses new matrix)
   - Job 4: Assemble charts (existing, uses new manifest)
   - Job 5: Catalog + deploy (existing)

3. **New script**: `.github/scripts/scan-patched-images.sh`
   - Post-Copa Trivy scanning for skip detection and attestation
   - Scans each patched image and saves report to `--patched-reports-dir`

4. **Simplify script**: `.github/scripts/assemble-charts.sh`
   - Remove pre-filter result merging (Copa handles this now)
   - Simplify to just call `verity assemble`

5. **Update commands**:
   - `cmd/catalog.go` - accept Copa results instead of values.yaml
   - `cmd/list.go` - read from Copa results
   - `internal/sitedata.go` - read image list from Copa results

### Files to Modify in Phase 3

| File | Change |
|------|--------|
| `.github/workflows/scan-and-patch.yaml` | Complete rewrite per plan section 5 |
| `.github/scripts/scan-patched-images.sh` | Create new (post-Copa scanning) |
| `.github/scripts/assemble-charts.sh` | Simplify (remove merging logic) |
| `cmd/catalog.go` | Accept Copa results dir instead of values.yaml |
| `cmd/list.go` | Read from Copa results |
| `internal/sitedata.go` | Read images from Copa results instead of values.yaml |

---

## 📦 Phase 4: Cleanup (Future)

Once Phase 3 is working in CI and validated:

### Files to Remove

- `cmd/scan.go` - Copa discovers images
- `cmd/discover.go` - Replaced by Copa + post-process
- `cmd/patch.go` - Copa handles all patching
- `internal/patcher.go` - Copa handles all patching
- `internal/filter.go` - Copa's `--patched-reports-dir` replaces pre-filtering
- `internal/filter_test.go` - Tests for removed code
- `internal/scanner.go` - Keep `Image` type, move to types file
- `.github/workflows/update-images.yaml` - No more chart scanning → values.yaml
- `.github/scripts/discover-images.sh` - Replaced by Copa bulk
- `charts/standalone/` - Images now in copa-config.yaml
- `values.yaml` - Replaced by copa-config.yaml

### Code to Keep (Unchanged)

- `internal/helm.go` - Wrapper chart assembly
- `internal/values.go` - Wrapper chart creation
- `internal/sbom.go` - Chart-level SBOMs
- `internal/matrix.go` - Wrapper chart assembly (reads manifest.json + results/)
- All signing/attestation scripts

---

## 🎯 Current Status

**Phase 2: ✅ COMPLETE**
- All code implemented and tested
- Ready for Phase 3 workflow integration

**Phase 1: ⏳ BLOCKED**
- Waiting for Copa PR #1475 to merge (upstream dependency)
- Can proceed with Phase 3 development using `--skip-digest-lookup` for testing

**Phase 3: 🔜 NEXT**
- Can start workflow rewrite now
- Test with mock Copa output initially
- Integrate with real Copa once PR merges

**Phase 4: 📅 FUTURE**
- Execute after Phase 3 is validated in production
- Remove deprecated commands and files
- Clean up codebase

---

## 📋 Migration Checklist

- [x] Create copa-config.yaml
- [x] Create chart-image-map.yaml
- [x] Implement internal/copaconfig.go
- [x] Implement internal/postprocess.go
- [x] Implement cmd/postprocess.go
- [x] Update main.go
- [x] Write comprehensive unit tests
- [x] Verify command works end-to-end
- [ ] Wait for Copa PR #1475 to merge
- [ ] Rewrite scan-and-patch.yaml workflow
- [ ] Create scan-patched-images.sh script
- [ ] Update catalog command
- [ ] Update list command
- [ ] Test full workflow in CI
- [ ] Remove deprecated code (Phase 4)

---

## 🧪 Local Testing

### Test post-process command

Create a test Copa output:
```json
{
  "results": [
    {
      "name": "nginx",
      "status": "Patched",
      "source_image": "docker.io/library/nginx:1.25.3",
      "patched_image": "ghcr.io/verity-org/library/nginx:1.25.3-patched",
      "details": "OK"
    }
  ]
}
```

Run post-process:
```bash
./verity post-process \
  --copa-output copa-output.json \
  --chart-map chart-image-map.yaml \
  --registry ghcr.io/verity-org \
  --output-dir .verity \
  --skip-digest-lookup
```

### Verify outputs

```bash
# Check matrix (should contain only patched images)
cat .verity/matrix.json | jq

# Check manifest (should group images by chart)
cat .verity/manifest.json | jq

# Check result files (one per image)
ls -la .verity/results/
```

---

## 📖 Architecture Changes

### Before (Current)
```
Renovate → Chart.yaml
  → verity scan → values.yaml
  → verity discover → manifest.json + matrix.json
  → GitHub Actions matrix (N jobs)
    → verity patch (Copa single-image) → result.json
  → verity assemble → wrapper charts
  → catalog → site
```

### After (New)
```
copa-config.yaml + chart-image-map.yaml (manual)
  → copa patch --config (bulk mode)
    → copa-output.json
  → verity post-process
    → manifest.json + matrix.json + results/*.json
  → GitHub Actions matrix (N jobs)
    → cosign sign + attest (no patching)
  → verity assemble → wrapper charts (unchanged)
  → catalog → site (unchanged)
```

### Key Improvements

1. **Version retention**: Pattern regex discovers all tags, old versions keep getting patched
2. **Simplification**: Copa handles discovery, skip detection, patching loop
3. **Performance**: Copa's bulk mode uses in-process workers (no shell per image)
4. **Reliability**: Built-in skip detection prevents redundant work
5. **Correctness**: Single source of truth (copa-config.yaml) instead of generated values.yaml

---

## 🚀 Ready for Phase 3!

Phase 2 implementation is complete and tested. You can now proceed with Phase 3 (workflow migration) or wait for Copa PR #1475 to merge if you want to test with real Copa output.
