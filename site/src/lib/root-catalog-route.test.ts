import assert from "node:assert/strict";
import test from "node:test";
import { getRootCatalogPaths, rootCatalogTarget } from "./root-catalog-route.ts";

test("root catalog routes redirect image repositories to their catalog pages", () => {
  assert.equal(rootCatalogTarget("caddy", "/"), "/catalog/caddy/");
  assert.equal(rootCatalogTarget("bazelbuild/bazel", "/"), "/catalog/bazelbuild/bazel/");
  assert.ok(getRootCatalogPaths().some(({ params }) => params.name === "caddy"));
});
