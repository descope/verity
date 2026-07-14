const GHCR = "https://ghcr.io";
const GHCR_ORGANIZATION = "verity-org";
const TOKEN_TTL_MS = 4 * 60 * 1000;
const TOKEN_REFRESH_SKEW_MS = 30 * 1000;
const MIN_TOKEN_CACHE_MS = 1_000;
const MAX_TOKEN_CACHE_ENTRIES = 64;

const tokenCache = new Map();
const repositoryPath = "[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*";
const digest = "sha256:[a-f0-9]{64}";
const artifactPath = new RegExp(
  `^/v2/(${repositoryPath})(/(?:manifests/[^/]+|blobs/${digest}|referrers/${digest}))$`
);
const forbiddenRepository = /^(?:charts|verity-org)(?:\/|$)/;
const forwardedHeaders = ["accept", "if-modified-since", "if-none-match", "range"];

export default {
  async fetch(request) {
    if (request.method !== "GET" && request.method !== "HEAD") {
      return noStoreResponse("Method not allowed", 405, { Allow: "GET, HEAD" });
    }

    const url = new URL(request.url);
    if (url.pathname === "/v2" || url.pathname === "/v2/") {
      return registryRoot();
    }

    const route = parseArtifactRoute(url.pathname);
    if (!route) {
      return noStoreResponse("Not found", 404);
    }

    try {
      const token = await anonymousPullToken(`${GHCR_ORGANIZATION}/${route.aliasRepository}`);
      const upstream = await fetch(`${GHCR}/v2/${GHCR_ORGANIZATION}/${route.aliasRepository}${route.suffix}${url.search}`, {
        method: request.method,
        headers: upstreamRequestHeaders(request.headers, token),
        redirect: "manual",
        cf: { cacheTtl: 0, cacheEverything: false },
      });
      return noStoreUpstreamResponse(upstream, request.method);
    } catch {
      return noStoreResponse("Bad gateway", 502);
    }
  },
};

/** Return the alias repository and matching OCI distribution suffix, if supported. */
export function parseArtifactRoute(pathname) {
  const match = artifactPath.exec(pathname);
  const aliasRepository = match?.[1];
  const suffix = match?.[2];
  if (!aliasRepository || !suffix || forbiddenRepository.test(aliasRepository)) {
    return null;
  }
  return { aliasRepository, suffix };
}

/** Keep a short-lived cache entry even when GHCR returns an unusually short token TTL. */
export function cacheLifetime(expiresInMs) {
  return Math.max(MIN_TOKEN_CACHE_MS, Math.min(expiresInMs - TOKEN_REFRESH_SKEW_MS, TOKEN_TTL_MS));
}

function registryRoot() {
  return new Response(null, {
    status: 200,
    headers: {
      "Cache-Control": "no-store",
      "Docker-Distribution-API-Version": "registry/2.0",
    },
  });
}

async function anonymousPullToken(repository) {
  const cached = tokenCache.get(repository);
  if (cached && cached.expiresAt > Date.now()) {
    return cached.token;
  }

  const tokenURL = new URL(`${GHCR}/token`);
  tokenURL.searchParams.set("service", "ghcr.io");
  tokenURL.searchParams.set("scope", `repository:${repository}:pull`);
  const response = await fetch(tokenURL, {
    headers: { Accept: "application/json" },
    redirect: "manual",
    cf: { cacheTtl: 0, cacheEverything: false },
  });
  if (!response.ok || response.status >= 300) {
    throw new Error(`GHCR token request failed with ${response.status}`);
  }

  const payload = await response.json();
  const token = typeof payload.token === "string" ? payload.token : payload.access_token;
  if (typeof token !== "string" || token === "") {
    throw new Error("GHCR token response did not include a bearer token");
  }

  const expiresInMs = typeof payload.expires_in === "number" ? payload.expires_in * 1000 : TOKEN_TTL_MS;
  tokenCache.delete(repository);
  if (tokenCache.size >= MAX_TOKEN_CACHE_ENTRIES) {
    const oldestRepository = tokenCache.keys().next().value;
    if (typeof oldestRepository === "string") {
      tokenCache.delete(oldestRepository);
    }
  }
  tokenCache.set(repository, { token, expiresAt: Date.now() + cacheLifetime(expiresInMs) });
  return token;
}

function upstreamRequestHeaders(requestHeaders, token) {
  const headers = new Headers();
  for (const name of forwardedHeaders) {
    const value = requestHeaders.get(name);
    if (value) {
      headers.set(name, value);
    }
  }
  headers.set("Accept", headers.get("Accept") ?? "*/*");
  headers.set("Authorization", `Bearer ${token}`);
  return headers;
}

function noStoreUpstreamResponse(upstream, requestMethod) {
  const headers = new Headers(upstream.headers);
  headers.delete("set-cookie");
  headers.delete("www-authenticate");
  headers.set("Cache-Control", "no-store");
  return new Response(requestMethod === "HEAD" ? null : upstream.body, { status: upstream.status, headers });
}

function noStoreResponse(message, status, additionalHeaders = {}) {
  return new Response(message, {
    status,
    headers: {
      "Cache-Control": "no-store",
      "Content-Type": "text/plain; charset=utf-8",
      ...additionalHeaders,
    },
  });
}
