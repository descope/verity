import rawData from '../data/catalog.json';

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
  standaloneImages: SiteImage[];
}

export const catalog: SiteData = rawData as SiteData;

export function getChartByName(name: string): SiteChart | undefined {
  return catalog.charts.find((c) => c.name === name);
}

export function getAllImages(): SiteImage[] {
  const chartImages = catalog.charts.flatMap((c) =>
    c.images.map((img) => ({ ...img, chartName: c.name }))
  );
  return [...chartImages, ...catalog.standaloneImages];
}

export function getImageById(id: string): SiteImage | undefined {
  return getAllImages().find((img) => img.id === id);
}

/** Short display name from a full image reference (last path segment + tag). */
export function shortName(ref: string): string {
  const withoutRegistry = ref.includes('/') ? ref.split('/').slice(1).join('/') : ref;
  return withoutRegistry;
}
