import assert from "node:assert/strict";
import test from "node:test";
import { getImagesFromCatalog } from "./catalog.ts";

test("getImagesFromCatalog returns an empty list when catalog images are absent", () => {
  assert.deepEqual(getImagesFromCatalog({}), []);
});
