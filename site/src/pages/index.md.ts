import type { APIRoute } from "astro";
import {
  fullCatalog,
  totalImages,
  totalCategories,
  copaCount,
  integerCount,
} from "../data/full-catalog";
import { getChartsCatalog } from "../lib/charts";

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;

  const chartsCatalog = getChartsCatalog();
  const chartCount = chartsCatalog.charts.length;

  const categorySummary = fullCatalog
    .map((cat) => {
      const copaImgs = cat.images.filter((i) => i.source === "copa").length;
      const wolfiImgs = cat.images.filter((i) => i.source === "integer").length;
      const parts = [];
      if (wolfiImgs > 0) parts.push(`${wolfiImgs} Wolfi`);
      if (copaImgs > 0) parts.push(`${copaImgs} Copa`);
      return `- **${cat.label}** — ${cat.images.length} images (${parts.join(", ")})`;
    })
    .join("\n");

  const content = `# Verity — Security-Patched Container Images

> Self-maintaining registry of security-patched container images. Scans, patches, signs, and publishes drop-in replacements to GitHub Container Registry.

## What Is Verity?

Container images accumulate CVEs daily. Upstream maintainers patch on their own schedule — if at all. Verity eliminates that trade-off by continuously scanning container images, patching them in-place using [Copa](https://github.com/project-copacetic/copacetic), and publishing signed, attested replacements.

**Registry**: \`ghcr.io/verity-org\`
**Pipeline**: Runs daily at 02:00 UTC via GitHub Actions
**Signing**: cosign keyless (Sigstore OIDC) + SLSA L3 provenance + CycloneDX SBOM

## Quick Start

Replace your image reference:

\`\`\`bash
docker pull ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
\`\`\`

\`\`\`yaml
# Kubernetes
image: ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched
\`\`\`

## Catalog Summary

${totalImages} images across ${totalCategories} categories — ${copaCount} Copa-patched upstream images, ${integerCount} Wolfi-based hardened images, ${chartCount} Helm wrapper charts.

${categorySummary}

## How It Works

1. **Discover** — Images from Helm charts and \`copa-config.yaml\`
2. **Scan** — Trivy detects known CVEs
3. **Patch** — Copa fixes OS packages in-place (no rebuild)
4. **Sign** — cosign + SLSA L3 + CycloneDX SBOM attestations
5. **Publish** — Pushed to \`ghcr.io/verity-org\`

## Supply Chain Attestations

Every image carries: cosign signature, SLSA L3 build provenance, CycloneDX SBOM, Trivy vulnerability report, and Rekor transparency log entry.

## More Information

- [Complete LLM Reference](${prefix}llms-full.txt) — Everything in one file
- [Supply Chain Compliance](${prefix}compliance.md) — Framework mappings (SLSA, FedRAMP, SOC 2, ISO 27001, OWASP)
- [Helm Charts](${prefix}charts/index.md) — Pre-patched wrapper Helm charts
- [GitHub Repository](https://github.com/verity-org/verity) — Source code and issues
`;

  return new Response(content.trim() + "\n", {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
