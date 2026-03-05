/**
 * Fetches integer catalog from the reports branch at build time.
 * This provides version information for integer (zero-CVE) images.
 *
 * Note: CATALOG_URL points to the mutable reports branch. For production,
 * consider pinning to a specific commit SHA or verifying the artifact.
 * EOL status is computed at build time and will update on each rebuild.
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

/** Cached result including registry from the catalog */
interface CatalogResult {
  images: IntegerImage[];
  registry: string;
}

const CATALOG_URL = "https://raw.githubusercontent.com/verity-org/integer/reports/catalog.json";

/** Cache the in-flight promise to prevent concurrent fetches during parallel SSG */
let cachedPromise: Promise<CatalogResult> | null = null;

const MAX_RETRIES = 3;
const RETRY_DELAY_MS = 1000;

async function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Fetches the integer catalog at build time with retry logic.
 * Returns an empty array if all retries fail (graceful degradation).
 * Uses promise caching to prevent duplicate fetches during parallel SSG builds.
 */
export async function getIntegerCatalog(): Promise<CatalogResult> {
  if (cachedPromise !== null) {
    return cachedPromise;
  }

  // Cache the promise immediately to prevent concurrent fetches
  cachedPromise = fetchWithRetry();
  return cachedPromise;
}

async function fetchWithRetry(): Promise<CatalogResult> {
  let lastError: unknown;

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    try {
      const response = await fetch(CATALOG_URL);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status} ${response.statusText}`);
      }

      const data: RawIntegerCatalog = await response.json();
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
    } catch (error) {
      lastError = error;
      if (attempt < MAX_RETRIES) {
        console.warn(
          `[integer-catalog] Fetch attempt ${attempt}/${MAX_RETRIES} failed, retrying in ${RETRY_DELAY_MS}ms...`
        );
        await sleep(RETRY_DELAY_MS);
      }
    }
  }

  console.warn(`[integer-catalog] All ${MAX_RETRIES} fetch attempts failed:`, lastError);
  // Don't cache failure - reset promise so next call can retry
  cachedPromise = null;
  return { images: [], registry: "" };
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
