import type { APIRoute } from "astro";
import { getChartsCatalog } from "../../lib/charts";

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;

  const chartsCatalog = getChartsCatalog();
  const charts = chartsCatalog.charts;

  const totalOverrides = charts.reduce(
    (sum, c) => sum + c.imageMappings.length + c.valueOverrides.length,
    0,
  );

  const chartsContent = charts.length > 0
    ? charts
        .map((chart) => {
          const installCmd = `helm install ${chart.wrapperName} ${chart.registry}/${chart.wrapperName} --version ${chart.wrapperVersion}`;
          const mappingCount = chart.imageMappings.length + chart.valueOverrides.length;

          let entry = `### ${chart.name} v${chart.version}\n\n`;
          entry += `- **Wrapper chart**: \`${chart.wrapperName}\` v${chart.wrapperVersion}\n`;
          entry += `- **Install**: \`${installCmd}\`\n`;
          entry += `- **Image overrides**: ${mappingCount}\n`;
          if (chart.repository) {
            entry += `- **Source**: \`${chart.repository}\`\n`;
          }

          if (chart.imageMappings.length > 0) {
            entry += "\n| Original | Patched |\n|----------|--------|\n";
            entry += chart.imageMappings
              .map(
                (m) =>
                  `| \`${m.originalRepo}:${m.originalTag}\` | \`${m.patchedRepo}:${m.patchedTag}\` |`,
              )
              .join("\n");
            entry += "\n";
          }

          if (chart.valueOverrides.length > 0) {
            entry += "\n**Value overrides:**\n\n";
            entry += chart.valueOverrides.map((v) => `- \`${v.path}\` → \`${v.value}\``).join("\n");
            entry += "\n";
          }

          return entry;
        })
        .join("\n")
    : "No wrapper charts have been generated yet. Charts are generated daily at 04:00 UTC after the patching pipeline completes.\n";

  const content = `# Helm Charts — Verity

> Pre-patched wrapper Helm charts that override upstream image references with Verity's security-patched equivalents. Install a chart and every container image is already patched.

## Summary

- **Charts**: ${charts.length}
- **Total image overrides**: ${totalOverrides}
- **Registry**: OCI (installable via \`helm install\`)

## How It Works

1. **Wrapper chart** — A thin Helm chart that declares the original chart as a dependency and overrides \`values.yaml\` to point image references at patched versions.
2. **OCI registry** — Wrapper charts are pushed to \`${chartsCatalog.chartRegistry || charts[0]?.registry || "oci://ghcr.io/verity-org/charts"}\` and can be installed directly via \`helm install\`.
3. **Drop-in replace** — Install the wrapper chart instead of the original. Helm resolves the dependency and applies all patched image overrides automatically.

## Available Charts

${chartsContent}

---

Charts generated daily at 04:00 UTC. [Full LLM Reference](${prefix}llms-full.txt) · [Browse Catalog](${prefix}) · [GitHub](https://github.com/verity-org/verity)
`;

  return new Response(content.trim() + "\n", {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
