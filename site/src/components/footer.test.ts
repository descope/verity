import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const vanityRegistryHref = /href="https:\/\/verity\.supply"/;
const githubPackagesHref = /github\.com\/orgs\/verity-org\/packages/;

test("footer registry link opens the vanity registry", () => {
  const footer = readFileSync(new URL("./Footer.astro", import.meta.url), "utf8");

  assert.match(footer, vanityRegistryHref);
  assert.doesNotMatch(footer, githubPackagesHref);
});
