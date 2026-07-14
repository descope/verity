# Verity vanity OCI registry

`worker.mjs` is a read-only Cloudflare Worker that exposes public Verity images
through `verity.supply/<image>` while preserving `ghcr.io/verity-org/<image>`
as the canonical registry.

The Worker never accepts pushes or client credentials. It obtains a short-lived
anonymous GHCR pull token for the namespaced upstream repository, then proxies
the requested artifact. Requests outside the OCI read surface return `404`.

## Supported OCI distribution requests

- `GET` and `HEAD` on `/v2/`
- image manifests, including legacy cosign signature tags
- `sha256` blobs
- OCI referrers, used by modern signature and attestation discovery

For example, the alias request below is resolved upstream as shown:

```text
verity.supply/caddy:latest
ghcr.io/verity-org/caddy:latest
```

`charts` and `verity-org` are deliberately reserved and cannot be used as alias
repositories. This prevents namespace confusion such as
`verity.supply/verity-org/caddy`.

## Deployment

Start the Worker locally with `npm run dev`, then deploy it with
`npx wrangler deploy`. `wrangler.toml` binds the Worker to
`verity.supply/v2/*`; the website continues to serve the remaining paths.

No secrets or environment variables are required. The Worker requests only
anonymous, repository-scoped pull tokens from GHCR and never forwards a client
`Authorization` header upstream.

Cloudflare security controls must allow OCI clients on `/v2/*`. A WAF or bot
rule that challenges Docker's manifest `HEAD` requests blocks the request
before it reaches this Worker.

## Verification

Run the unit tests and syntax check locally:

```bash
cd registry
npm test
npm run check
```

With `npm run dev` running, the local registry endpoint is available at
`http://127.0.0.1:8787/v2/` for HTTP-level checks.

After deployment, verify an alias through an OCI client:

```bash
docker pull verity.supply/caddy:latest
```

The Worker exposes the manifest, legacy cosign signature-tag, and OCI-referrer
surfaces used by signature and attestation tooling. Verification also requires
that the image has an associated signature or attestation. If a tool only
follows GitHub's attestation API, use the canonical image reference instead:

```bash
gh attestation verify oci://ghcr.io/verity-org/caddy:latest --owner verity-org
```
