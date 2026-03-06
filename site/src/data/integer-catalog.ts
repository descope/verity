import { readFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import process from "node:process";
import type { IntegerImage, IntegerVariant, IntegerVersion } from "../lib/catalog";

interface RawIntegerCatalog {
  generatedAt: string;
  registry: string;
  images: Array<{
    name: string;
    description: string;
    versions: Array<{
      version: string;
      latest?: boolean;
      eol?: string;
      variants: Array<{
        type: string;
        tags: string[];
        ref: string;
        digest: string;
        builtAt: string;
        status: "success" | "failure" | "unknown";
      }>;
    }>;
  }>;
}

interface CatalogResult {
  images: IntegerImage[];
  registry: string;
}

const CATALOG_PATH = resolve(process.cwd(), "src/data/integer-catalog.json");

let cached: CatalogResult | null = null;

function loadCatalog(): CatalogResult {
  if (!existsSync(CATALOG_PATH)) {
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
    console.warn(`[integer-catalog] Failed to parse ${CATALOG_PATH}:`, err);
    return { images: [], registry: "" };
  }

  const images = data.images.map((img) => ({
    name: img.name,
    description: img.description,
    versions: img.versions.map(
      (v): IntegerVersion => ({
        version: v.version,
        latest: v.latest,
        eol: v.eol,
        variants: v.variants.map(
          (r): IntegerVariant => ({
            type: r.type,
            tags: r.tags,
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

export async function getIntegerCatalog(): Promise<CatalogResult> {
  if (cached !== null) return cached;
  cached = loadCatalog();
  return cached;
}

interface IntegerImageWithRegistry extends IntegerImage {
  registry: string;
}

export async function getIntegerImage(name: string): Promise<IntegerImageWithRegistry | undefined> {
  const { images, registry } = await getIntegerCatalog();
  const image = images.find((img) => img.name === name);
  if (!image) return undefined;
  return { ...image, registry };
}
