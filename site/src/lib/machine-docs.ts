import { fullCatalog } from "../data/full-catalog.ts";

export function renderCategoryRows(): string {
  return fullCatalog
    .map((category) => {
      const copaImages = category.images.filter((image) => image.source === "copa").length;
      const wolfiImages = category.images.length - copaImages;
      return `  ${category.id},${category.images.length},${copaImages},${wolfiImages}`;
    })
    .join("\n");
}
