import type { APIRoute } from "astro";
import { copaCount, integerCount, totalCategories, totalImages } from "../data/full-catalog.ts";
import { getChartsCatalog } from "../lib/charts.ts";
import { renderCategoryRows } from "../lib/machine-docs.ts";

export function renderIndexMarkdown(prefix: string): string {
  const chartCount = getChartsCatalog().charts.length;
  const categoryRows = renderCategoryRows();

  return `# Verity — Security-Patched Container Images

> Self-maintaining registry of security-patched container images. Scans, patches, signs, and publishes drop-in replacements to \`verity.supply\`.

scope: overview
total_images: ${totalImages}
total_categories: ${totalCategories}
copa_images: ${copaCount}
wolfi_images: ${integerCount}
helm_charts: ${chartCount}
full_reference: ${prefix}llms-full.txt

## What Is Verity?

Container images accumulate CVEs daily. Upstream maintainers patch on their own schedule — if at all. Verity eliminates that trade-off by continuously scanning container images, patching them in-place using [Copa](https://github.com/project-copacetic/copacetic), and publishing signed, attested replacements.

**Registry**: \`verity.supply\`
**Pipeline**: Runs daily at 02:00 UTC via GitHub Actions
**Signing**: cosign keyless (Sigstore OIDC) + SLSA L3 provenance + CycloneDX SBOM

## Quick Start

Replace your image reference:

\`\`\`bash
docker pull verity.supply/prometheus/prometheus:v3.9.1-patched
\`\`\`

\`\`\`yaml
# Kubernetes
image: verity.supply/prometheus/prometheus:v3.9.1-patched
\`\`\`

## Catalog Summary

${totalImages} images across ${totalCategories} categories — ${copaCount} Copa-patched upstream images, ${integerCount} Wolfi-based hardened images, ${chartCount} Helm wrapper charts.

categories[${totalCategories}]{category,total,copa,wolfi}:
${categoryRows}

## How It Works

1. **Discover** — Images from Helm charts and \`copa-config.yaml\`
2. **Scan** — Trivy detects known CVEs
3. **Patch** — Copa fixes OS packages in-place (no rebuild)
4. **Sign** — cosign + SLSA L3 + CycloneDX SBOM attestations
5. **Publish** — Pushed to \`verity.supply\`

## Supply Chain Attestations

Every image carries: cosign signature, SLSA L3 build provenance, CycloneDX SBOM, Trivy vulnerability report, and Rekor transparency log entry.

## Next Actions

- [Browse the live catalog](${prefix}) — Search images and inspect current vulnerability data
- [Complete LLM Reference](${prefix}llms-full.txt) — Everything in one file
- [Supply Chain Compliance](${prefix}compliance.md) — Framework mappings (SLSA, FedRAMP, SOC 2, ISO 27001, OWASP)
- [Helm Charts](${prefix}charts/index.md) — Pre-patched wrapper Helm charts
- [Experimental APK Repository](${prefix}apk/index.md) — Pending package repository metadata, install flow, trust model, and supported arches
- [GitHub Repository](https://github.com/verity-org/verity) — Source code and issues
`;
}

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;
  const content = renderIndexMarkdown(prefix);

  return new Response(`${content.trim()}\n`, {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
