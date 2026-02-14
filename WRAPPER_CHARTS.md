# Wrapper Charts

When running verity in patch mode (`-patch`), it creates wrapper charts that make it easy for users to consume your patched images while maintaining full customization capabilities.

## How It Works

For each dependency in your Chart.yaml, verity creates a wrapper chart with the naming pattern `<chart-name>-verity`. This wrapper chart:

1. **Subcharts the original chart** - The wrapper declares the original chart as a dependency
2. **Provides patched images** - Values are pre-configured to use Copa-patched images
3. **Allows full customization** - Users can override any value, just like the original chart

## Example

Given this Chart.yaml dependency:

```yaml
dependencies:
  - name: prometheus
    version: "25.8.0"
    repository: oci://ghcr.io/prometheus-community/charts
```

Running verity with patching:

```bash
./verity -chart Chart.yaml -output charts -patch \
  -registry ghcr.io/descope/verity \
  -buildkit-addr docker-container://buildkitd
```

Creates this structure:

```
charts/
  prometheus-verity/
    Chart.yaml          # Wrapper chart that depends on prometheus
    values.yaml         # Patched image references
    .helmignore        # Standard Helm ignore patterns
```

### Generated Chart.yaml

```yaml
apiVersion: v2
name: prometheus-verity
description: prometheus with Copa-patched container images
type: application
version: 1.0.0
dependencies:
  - name: prometheus
    version: "25.8.0"
    repository: oci://ghcr.io/prometheus-community/charts
```

### Generated values.yaml

```yaml
prometheus:
  server:
    image:
      registry: ghcr.io/descope/verity
      repository: prometheus
      tag: v2.48.0-patched
  alertmanager:
    image:
      registry: ghcr.io/descope/verity
      repository: alertmanager
      tag: v0.26.0-patched
  # ... other patched images
```

## Using Wrapper Charts

### Install with default patched images

```bash
helm dependency build charts/prometheus-verity/
helm install my-prometheus charts/prometheus-verity/
```

### Install with custom values

Create your custom values file:

```yaml
# my-values.yaml
prometheus:
  server:
    replicaCount: 3  # Your customization
    resources:
      requests:
        cpu: 500m
        memory: 2Gi
```

The patched images from the wrapper chart's values.yaml will be merged with your custom values:

```bash
helm dependency build charts/prometheus-verity/
helm install my-prometheus charts/prometheus-verity/ -f my-values.yaml
```

### How Value Merging Works

Helm merges values in this order (later overrides earlier):

1. Default values from prometheus chart
2. **Patched image values from prometheus-verity/values.yaml**
3. Your custom values from `-f my-values.yaml`

This means:
- ✅ You get patched images automatically
- ✅ You can customize any prometheus setting
- ✅ You can even override patched images if needed

## Benefits

### For Chart Maintainers
- Provide security-patched images without forking upstream charts
- Update to new chart versions independently of patching
- Publish wrapper charts to your own registry

### For Chart Consumers
- Drop-in replacement for original charts
- Same customization options as original
- Transparent security patching
- Easy to switch back to original if needed

## Advanced Usage

### Publishing Wrapper Charts

You can package and publish wrapper charts to your OCI registry:

```bash
# Build dependencies
helm dependency build charts/prometheus-verity/

# Package the chart
helm package charts/prometheus-verity/

# Push to your registry
helm push prometheus-verity-1.0.0.tgz oci://ghcr.io/your-org/verity/charts
```

Then users can install directly from your registry:

```bash
helm install my-prometheus oci://ghcr.io/your-org/verity/charts/prometheus-verity --version 1.0.0
```

### Overriding Patched Images

If you need to use a specific image:

```yaml
# my-values.yaml
prometheus:
  server:
    image:
      registry: my-registry.com
      repository: custom/prometheus
      tag: my-version
```
