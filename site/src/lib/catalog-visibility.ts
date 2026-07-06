import type { FullCatalogCategory, FullCatalogImage } from "../data/full-catalog.ts";
import type { VulnSummary } from "./catalog.ts";

const JAVA_STACK_NAMES = new Set([
  "library/gradle",
  "maven",
  "library/elasticsearch",
  "opensearchproject/opensearch",
  "opensearchproject/opensearch-dashboards",
  "confluentinc/cp-kafka",
  "strimzi/kafka",
  "kafka",
  "library/zookeeper",
  "opensearchproject/logstash-oss-with-opensearch-output-plugin",
  "keycloak/keycloak",
  "mlflow/mlflow",
  "library/sonarqube",
]);

export function shouldShowCatalogImage(image: FullCatalogImage, vulnSummary: VulnSummary): boolean {
  if (!JAVA_STACK_NAMES.has(image.name)) {
    return true;
  }
  return (vulnSummary.severityCounts.CRITICAL ?? 0) === 0 && (vulnSummary.severityCounts.HIGH ?? 0) === 0;
}

export function filterCatalogCategories(
  categories: FullCatalogCategory[],
  getVulnSummary: (image: FullCatalogImage) => VulnSummary,
): FullCatalogCategory[] {
  return categories.flatMap((category) => {
    const images = category.images.filter((image) => shouldShowCatalogImage(image, getVulnSummary(image)));
    return images.length > 0 ? [{ ...category, images }] : [];
  });
}
