import rawData from "../data/catalog.json";

export interface VulnSummary {
  total: number;
  fixable: number;
  severityCounts: Record<string, number>;
}

export interface SiteVuln {
  id: string;
  pkgName: string;
  installedVersion: string;
  fixedVersion: string;
  severity: string;
  title: string;
}

export interface SiteImage {
  id: string;
  originalRef: string;
  patchedRef: string;
  valuesPath: string;
  os: string;
  overriddenFrom?: string;
  vulnSummary: VulnSummary;
  vulnerabilities: SiteVuln[];
  chartName?: string;
}

export interface SiteChart {
  name: string;
  version: string;
  upstreamVersion: string;
  description: string;
  repository: string;
  helmInstall: string;
  images: SiteImage[];
}

export interface SiteSummary {
  totalCharts: number;
  totalImages: number;
  totalVulns: number;
  fixableVulns: number;
}

export interface SiteData {
  generatedAt: string;
  registry: string;
  summary: SiteSummary;
  charts: SiteChart[];
  images: SiteImage[];
  /** @deprecated Use `images` instead. Kept for backward compatibility. */
  standaloneImages: SiteImage[];
}

export const catalog: SiteData = rawData as SiteData;

const charts = catalog.charts ?? [];

export function getChartByName(name: string): SiteChart | undefined {
  return charts.find((c) => c.name === name);
}

/** Returns unique chart names across all versions. */
export function getChartNames(): string[] {
  return [...new Set(charts.map((c) => c.name))];
}

/** Returns all chart versions for a given chart name, newest first. */
export function getChartVersions(name: string): SiteChart[] {
  return charts.filter((c) => c.name === name);
}

/** Returns a specific chart by name and version. */
export function getChartVersion(name: string, version: string): SiteChart | undefined {
  return charts.find((c) => c.name === name && c.version === version);
}

export function getAllImages(): SiteImage[] {
  const chartImages = charts.flatMap((c) =>
    c.images.map((img) => ({ ...img, chartName: c.name }))
  );
  // Support both the new `images` field and the deprecated `standaloneImages`.
  const extra = catalog.images ?? catalog.standaloneImages ?? [];
  return [...chartImages, ...extra];
}

export function getImageById(id: string): SiteImage | undefined {
  return getAllImages().find((img) => img.id === id);
}

/** Short display name from a full image reference (last path segment + tag). */
export function shortName(ref: string): string {
  const withoutRegistry = ref.includes("/") ? ref.split("/").slice(1).join("/") : ref;
  return withoutRegistry;
}
