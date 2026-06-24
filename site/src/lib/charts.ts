import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import process from "node:process";

export interface ChartImageMapping {
  originalRepo: string;
  originalTag: string;
  patchedRepo: string;
  patchedTag: string;
}

export interface ChartValueOverride {
  path: string;
  value: string;
}

export interface ChartEntry {
  name: string;
  version: string;
  wrapperName: string;
  wrapperVersion: string;
  repository: string;
  registry: string;
  imageMappings: ChartImageMapping[];
  valueOverrides: ChartValueOverride[];
}

export interface ChartsCatalog {
  generatedAt: string;
  chartRegistry: string;
  charts: ChartEntry[];
}

const CATALOG_PATH = resolve(process.cwd(), "src/data/charts-catalog.json");
let cached: ChartsCatalog | null = null;

export function getChartsCatalog(): ChartsCatalog {
  if (cached !== null) return cached;
  if (!existsSync(CATALOG_PATH)) {
    // eslint-disable-next-line no-console
    console.warn(`[charts-catalog] ${CATALOG_PATH} not found`);
    return { generatedAt: "", chartRegistry: "", charts: [] };
  }
  try {
    const raw = readFileSync(CATALOG_PATH, "utf-8");
    cached = JSON.parse(raw) as ChartsCatalog;
    return cached;
  } catch {
    // eslint-disable-next-line no-console
    console.warn(`[charts-catalog] Failed to parse ${CATALOG_PATH}`);
    return { generatedAt: "", chartRegistry: "", charts: [] };
  }
}
