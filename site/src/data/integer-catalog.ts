import { existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import process from "node:process";
import type { IntegerImage, IntegerVariant, IntegerVersion } from "../lib/catalog.ts";

interface RawIntegerCatalog {
  generatedAt: string;
  registry: string;
  images?: Array<{
    name: string;
    description: string;
    versions?: Array<{
      version: string;
      latest?: boolean;
      eol?: string;
      variants?: Array<{
        type: string;
        tags?: string[] | null;
        ref: string;
        digest: string;
        builtAt: string;
        status: "success" | "failure" | "unknown";
      }> | null;
    }> | null;
  }> | null;
}

interface CatalogResult {
  images: IntegerImage[];
  registry: string;
}

const CATALOG_PATH = resolve(process.cwd(), "src/data/integer-catalog.json");

let cached: CatalogResult | null = null;

function loadCatalog(): CatalogResult {
  if (!existsSync(CATALOG_PATH)) {
    // biome-ignore lint/suspicious/noConsole: Missing generated catalog should explain the local generation command.
    console.warn(
      `[integer-catalog] ${CATALOG_PATH} not found — run: ./verity integer catalog --output site/src/data/integer-catalog.json`
    );
    return { images: [], registry: "" };
  }

  let data: RawIntegerCatalog;
  try {
    const raw = readFileSync(CATALOG_PATH, "utf-8");
    data = JSON.parse(raw);
  } catch (err) {
    // biome-ignore lint/suspicious/noConsole: Parse failures should include the generated catalog path.
    console.warn(`[integer-catalog] Failed to parse ${CATALOG_PATH}:`, err);
    return { images: [], registry: "" };
  }

  const images = (data.images ?? []).map((img) => ({
    name: img.name,
    description: img.description,
    versions: (img.versions ?? []).map(
      (v): IntegerVersion => ({
        version: v.version,
        latest: v.latest,
        eol: v.eol,
        variants: (v.variants ?? []).map(
          (r): IntegerVariant => ({
            type: r.type,
            tags: r.tags ?? [],
            ref: r.ref,
            digest: r.digest,
            builtAt: r.builtAt,
            status: r.status,
          })
        ),
      })
    ),
  }));

  return { images, registry: data.registry };
}

export function getIntegerCatalog(): CatalogResult {
  if (cached !== null) {
    return cached;
  }
  cached = loadCatalog();
  return cached;
}

interface IntegerImageWithRegistry extends IntegerImage {
  registry: string;
}

export function getIntegerImage(name: string): IntegerImageWithRegistry | undefined {
  const { images, registry } = getIntegerCatalog();
  const image = images.find((img) => img.name === name);
  if (!image) {
    return;
  }
  return { ...image, registry };
}
