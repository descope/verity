/**
 * Fetches integer catalog from the reports branch at build time.
 * This provides version information for integer (zero-CVE) images.
 */
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

const CATALOG_URL =
  "https://raw.githubusercontent.com/verity-org/integer/reports/catalog.json";

let cachedCatalog: IntegerImage[] | null = null;

/**
 * Fetches the integer catalog at build time.
 * Returns an empty array if fetch fails (graceful degradation).
 */
export async function getIntegerCatalog(): Promise<IntegerImage[]> {
  if (cachedCatalog !== null) {
    return cachedCatalog;
  }

  try {
    const response = await fetch(CATALOG_URL);
    if (!response.ok) {
      console.warn(
        `[integer-catalog] Failed to fetch: ${response.status} ${response.statusText}`
      );
      cachedCatalog = [];
      return cachedCatalog;
    }

    const data: RawIntegerCatalog = await response.json();
    cachedCatalog = data.images.map((img) => ({
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

    return cachedCatalog;
  } catch (error) {
    console.warn("[integer-catalog] Error fetching catalog:", error);
    cachedCatalog = [];
    return cachedCatalog;
  }
}

/**
 * Finds an integer image by name.
 */
export async function getIntegerImage(
  name: string
): Promise<IntegerImage | undefined> {
  const catalog = await getIntegerCatalog();
  return catalog.find((img) => img.name === name);
}
