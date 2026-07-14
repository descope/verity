import assert from "node:assert/strict";
import test from "node:test";
import { toPublicRef } from "./refs.ts";

test("toPublicRef maps canonical Verity image references to the vanity registry", () => {
  assert.equal(toPublicRef("ghcr.io/verity-org/caddy:latest"), "verity.supply/caddy:latest");
  assert.equal(
    toPublicRef("oci://ghcr.io/verity-org/charts/prometheus"),
    "oci://verity.supply/charts/prometheus"
  );
  assert.equal(
    toPublicRef("quay.io/prometheus/prometheus:v3.9.1"),
    "quay.io/prometheus/prometheus:v3.9.1"
  );
});
