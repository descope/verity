import assert from "node:assert/strict";
import test from "node:test";
import worker, { cacheLifetime, parseArtifactRoute } from "./worker.mjs";

test("parseArtifactRoute maps an alias manifest request to the Verity GHCR namespace", () => {
  assert.deepEqual(parseArtifactRoute("/v2/caddy/manifests/latest"), {
    aliasRepository: "caddy",
    suffix: "/manifests/latest",
  });
});

test("parseArtifactRoute permits OCI referrer discovery but blocks canonical namespace aliases", () => {
  assert.deepEqual(
    parseArtifactRoute(`/v2/caddy/referrers/${"sha256:" + "a".repeat(64)}`),
    {
      aliasRepository: "caddy",
      suffix: `/referrers/${"sha256:" + "a".repeat(64)}`,
    }
  );
  assert.equal(parseArtifactRoute("/v2/verity-org/caddy/manifests/latest"), null);
});

test("cacheLifetime retains tokens with short expiry windows", () => {
  assert.equal(cacheLifetime(10_000), 1_000);
  assert.equal(cacheLifetime(300_000), 240_000);
});

test("worker proxies alias manifests with a namespaced GHCR token", async () => {
  const fetch = globalThis.fetch;
  const calls = [];
  globalThis.fetch = async (input, init = {}) => {
    calls.push({ url: String(input), init });
    if (String(input).startsWith("https://ghcr.io/token?")) {
      return Response.json({ token: "pull-token", expires_in: 300 });
    }
    return new Response("manifest", { headers: { "Docker-Content-Digest": "sha256:test" } });
  };

  try {
    const response = await worker.fetch(
      new Request("https://verity.supply/v2/caddy/manifests/latest", {
        headers: { Authorization: "Bearer client-token", Accept: "application/vnd.oci.image.manifest.v1+json" },
      })
    );

    assert.equal(response.status, 200);
    assert.equal(await response.text(), "manifest");
    assert.equal(calls.length, 2);
    assert.match(calls[0].url, /scope=repository%3Averity-org%2Fcaddy%3Apull/);
    assert.equal(calls[1].url, "https://ghcr.io/v2/verity-org/caddy/manifests/latest");
    assert.equal(calls[1].init.headers.get("Authorization"), "Bearer pull-token");
    assert.equal(calls[1].init.headers.get("Accept"), "application/vnd.oci.image.manifest.v1+json");
  } finally {
    globalThis.fetch = fetch;
  }
});

test("worker synthesizes a bodyless HEAD response from an upstream manifest GET", async () => {
  const fetch = globalThis.fetch;
  const calls = [];
  globalThis.fetch = async (input, init = {}) => {
    calls.push({ url: String(input), init });
    if (String(input).startsWith("https://ghcr.io/token?")) {
      return Response.json({ token: "pull-token", expires_in: 300 });
    }
    return new Response("manifest", { headers: { "Docker-Content-Digest": "sha256:test" } });
  };

  try {
    const response = await worker.fetch(
      new Request("https://verity.supply/v2/caddy-head/manifests/latest", { method: "HEAD" })
    );

    assert.equal(response.status, 200);
    assert.equal(await response.text(), "");
    assert.equal(calls[1].init.method, "GET");
    assert.equal(response.headers.get("Docker-Content-Digest"), "sha256:test");
  } finally {
    globalThis.fetch = fetch;
  }
});
