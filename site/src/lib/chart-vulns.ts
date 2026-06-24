import type { VulnSummary } from "./catalog.ts";
import { catalog } from "./catalog.ts";
import type { ChartEntry } from "./charts.ts";

const EMPTY_VULNS: VulnSummary = { total: 0, severityCounts: {} };

const vulnMap = new Map<string, VulnSummary>();
for (const img of catalog.images) {
  vulnMap.set(img.patchedRef, img.afterVulns);
}

export function getImageVulns(patchedRepo: string, patchedTag: string): VulnSummary {
  return vulnMap.get(`${patchedRepo}:${patchedTag}`) ?? EMPTY_VULNS;
}

export function getChartVulns(chart: ChartEntry): VulnSummary {
  const merged: Record<string, number> = {};
  let total = 0;
  for (const m of chart.imageMappings) {
    const v = getImageVulns(m.patchedRepo, m.patchedTag);
    total += v.total;
    for (const [sev, count] of Object.entries(v.severityCounts)) {
      merged[sev] = (merged[sev] ?? 0) + count;
    }
  }
  return { total, severityCounts: merged };
}
