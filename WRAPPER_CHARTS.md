# Wrapper Charts (Deprecated)

> **⚠️ This approach is no longer used.**
> Verity now uses a single `Chart.yaml` with dependencies and a centralized `values.yaml` instead of
> individual wrapper charts.

## Migration

**Old approach:**

```text
charts/
  prometheus/
    Chart.yaml    # Wrapper chart
    values.yaml   # Patched images
  postgres-operator/
    Chart.yaml
    values.yaml
```

**New approach:**

```text
Chart.yaml        # Single meta-chart with all dependencies
values.yaml       # Centralized image list (auto-generated)
Chart.lock        # Locked dependency versions
charts/*.tgz      # Downloaded dependencies (gitignored)
```

## Benefits of New Approach

✅ **Simpler** - One Chart.yaml vs many
✅ **Automated** - Renovate updates Chart.yaml → scan updates values.yaml
✅ **Centralized** - All images in one file
✅ **No publishing** - Users pull from ghcr.io/verity-org directly

## How It Works Now

1. **Chart.yaml** defines chart dependencies (Renovate updates these)
2. **verity scan** extracts images from all charts
3. **values.yaml** contains all discovered images
4. **verity discover** parses values.yaml and applies overrides
5. **Workflow** patches images in parallel to GHCR

See [WORKFLOWS.md](WORKFLOWS.md) for current architecture.

## Using Patched Images

Instead of installing wrapper charts, users pull patched images directly:

```bash
# Old (wrapper charts)
helm install prometheus oci://ghcr.io/verity-org/charts/prometheus

# New (patched images)
docker pull ghcr.io/verity-org/prometheus/prometheus:v2.45.0-patched

# Or in Helm values
prometheus:
  server:
    image:
      registry: ghcr.io/verity-org
      repository: prometheus/prometheus
      tag: v2.45.0-patched
```

## Why the Change?

**Wrapper charts were complex:**

- Required publishing charts to OCI registry
- Needed version management (upstream-version + patch-level)
- Users had to use special wrapper charts

**New approach is simpler:**

- Images are the primary artifact
- No chart publishing needed
- Users can use upstream charts with our images
- Renovate integration is cleaner
