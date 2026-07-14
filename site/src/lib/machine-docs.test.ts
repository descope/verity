import assert from "node:assert/strict";
import test from "node:test";
import { totalCategories } from "../data/full-catalog.ts";
import { renderIndexMarkdown } from "../pages/index.md.ts";
import { renderLlmsText } from "../pages/llms.txt.ts";
import { renderCategoryRows } from "./machine-docs.ts";

const PREFIX = "https://example.test/";
const CATEGORY_HEADER_PATTERN = /^categories\[14\]\{category,total,copa,wolfi\}:$/m;

test("renderLlmsText gives a concise live summary with a full-data escape hatch", () => {
  // Given
  const prefix = PREFIX;

  // When
  const output = renderLlmsText(prefix);

  // Then
  assert.ok(output.includes("scope: summary"));
  assert.ok(output.includes("total_images: 314"));
  assert.ok(output.includes("total_categories: 14"));
  assert.ok(output.includes("full_reference: https://example.test/llms-full.txt"));
  assert.match(output, CATEGORY_HEADER_PATTERN);
  assert.ok(output.includes("next[6]{task,url}:"));
  assert.ok(!output.includes("/axi"));
});

test("renderIndexMarkdown exposes aggregates and contextual next actions", () => {
  // Given
  const prefix = PREFIX;

  // When
  const output = renderIndexMarkdown(prefix);

  // Then
  assert.ok(output.includes("scope: overview"));
  assert.ok(output.includes("total_images: 314"));
  assert.ok(output.includes("full_reference: https://example.test/llms-full.txt"));
  assert.match(output, CATEGORY_HEADER_PATTERN);
  assert.ok(output.includes("## Next Actions"));
  assert.ok(!output.includes("Agent Interface"));
});

test("machine doc routes share the catalog category rows", () => {
  // Given
  const categoryRows = renderCategoryRows();

  // When
  const llmsText = renderLlmsText(PREFIX);
  const indexMarkdown = renderIndexMarkdown(PREFIX);

  // Then
  assert.equal(categoryRows.split("\n").length, totalCategories);
  assert.ok(llmsText.includes(categoryRows));
  assert.ok(indexMarkdown.includes(categoryRows));
});
