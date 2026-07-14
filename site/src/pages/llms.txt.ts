import type { APIRoute } from "astro";
import { copaCount, integerCount, totalCategories, totalImages } from "../data/full-catalog.ts";
import { renderCategoryRows } from "../lib/machine-docs.ts";

export function renderLlmsText(prefix: string): string {
  const categoryRows = renderCategoryRows();

  return `# Verity

> Self-maintaining registry of security-patched container images. Verity scans, patches, signs, attests, and publishes drop-in replacements at verity.supply.

scope: summary
total_images: ${totalImages}
total_categories: ${totalCategories}
copa_images: ${copaCount}
wolfi_images: ${integerCount}
full_reference: ${prefix}llms-full.txt

## Quick Start

\`\`\`
docker pull verity.supply/prometheus/prometheus:v3.9.1-patched
\`\`\`

## Catalog by Category

categories[${totalCategories}]{category,total,copa,wolfi}:
${categoryRows}

## Operational Facts

- Platforms: linux/amd64 and linux/arm64
- Pipeline: daily at 02:00 UTC and on configuration changes
- Evidence: cosign signature, SLSA L3 provenance, CycloneDX SBOM, Trivy report, Rekor entry
- FIPS-capable variants: selected images; see the full reference for the complete list

## Next Actions

next[6]{task,url}:
  browse_catalog,${prefix}
  overview,${prefix}index.md
  full_reference,${prefix}llms-full.txt
  compliance,${prefix}compliance.md
  helm_charts,${prefix}charts/index.md
  apk_repository,${prefix}apk/index.md
`;
}

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;
  const content = renderLlmsText(prefix);

  return new Response(`${content.trim()}\n`, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
