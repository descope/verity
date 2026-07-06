import assert from "node:assert/strict";
import test from "node:test";
import type { FullCatalogCategory } from "../data/full-catalog.ts";
import { filterCatalogCategories, shouldShowCatalogImage } from "./catalog-visibility.ts";
import { getImagesFromCatalog } from "./catalog.ts";

test("getImagesFromCatalog returns an empty list when catalog images are absent", () => {
  assert.deepEqual(getImagesFromCatalog({}), []);
});

test("shouldShowCatalogImage hides Java stack images with blocked vulnerabilities", () => {
  assert.equal(
    shouldShowCatalogImage(
      { name: "library/sonarqube", source: "copa" },
      { total: 1, severityCounts: { CRITICAL: 1 } },
    ),
    false,
  );
  assert.equal(shouldShowCatalogImage({ name: "library/sonarqube", source: "copa" }, { total: 0, severityCounts: {} }), true);
  assert.equal(
    shouldShowCatalogImage({ name: "airflow", source: "integer" }, { total: 3, severityCounts: { HIGH: 3 } }),
    true,
  );
});

test("filterCatalogCategories removes empty categories after Java stack filtering", () => {
  const categories: FullCatalogCategory[] = [
    { id: "java", label: "Java", images: [{ name: "library/sonarqube", source: "copa" }] },
    { id: "runtime", label: "Runtime", images: [{ name: "airflow", source: "integer" }] },
  ];
  const filtered = filterCatalogCategories(categories, (image) => ({
    total: image.name === "library/sonarqube" ? 1 : 0,
    severityCounts: image.name === "library/sonarqube" ? { HIGH: 1 } : {},
  }));
  assert.deepEqual(
    filtered.map((category) => category.id),
    ["runtime"],
  );
});
