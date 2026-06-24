import rawData from "../data/catalog.json" with { type: "json" };

export interface VulnSummary {
  total: number;
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
  os: string;
  beforeVulns: VulnSummary;
  afterVulns: VulnSummary;
  vulnerabilities: SiteVuln[];
}

export interface SiteSummary {
  totalImages: number;
  totalVulnsBefore: number;
  totalVulnsAfter: number;
  fixedVulns: number;
}

export interface IntegerVariant {
  type: string;
  tags: string[];
  ref: string;
  digest: string;
  builtAt: string;
  status: "success" | "failure" | "unknown";
}

export interface IntegerVersion {
  version: string;
  latest?: boolean;
  eol?: string;
  variants: IntegerVariant[];
}

export interface IntegerImage {
  name: string;
  description: string;
  versions: IntegerVersion[];
}

export interface SiteData {
  generatedAt: string;
  registry: string;
  summary: SiteSummary;
  images: SiteImage[];
  integerImages?: IntegerImage[];
}

export const catalog: SiteData = rawData as SiteData;

export function getAllImages(): SiteImage[] {
  return catalog.images;
}

export function getImageById(id: string): SiteImage | undefined {
  return getAllImages().find((img) => img.id === id);
}

/** Short display name from a full image reference (last path segment + tag). */
export function shortName(ref: string): string {
  const withoutRegistry = ref.includes("/") ? ref.split("/").slice(1).join("/") : ref;
  return withoutRegistry;
}
