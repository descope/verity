import { fullCatalog } from "../data/full-catalog.ts";

export function getRootCatalogPaths() {
  return fullCatalog.flatMap((category) =>
    category.images.map((image) => ({ params: { name: image.name } }))
  );
}

export function rootCatalogTarget(name: string, base: string): string {
  return `${base}catalog/${name}/`;
}
