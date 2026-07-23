import type { APIRoute } from "astro";
import { apkRepository } from "../../data/apk-repository.ts";

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;
  const repoRoot = `${prefix}${apkRepository.basePath}`;
  const keyUrl = `${repoRoot}/${apkRepository.keyFile}`;

  const archRows = apkRepository.architectures
    .map(
      (arch) =>
        `| \`${arch.apk}\` | \`${arch.platform}\` | \`${repoRoot}/${arch.apk}\` | \`${repoRoot}/${arch.apk}/APKINDEX.tar.gz\` |`
    )
    .join("\n");

  const content = `# Experimental APK Repository — Verity

> Status: **${apkRepository.status}**. ${apkRepository.caveat}

The Verity APK repository is the package-level companion to the container-image catalog. It exposes approved Verity-built APK packages for Wolfi/Alpine-compatible consumers.

## Repository Entry Points

| APK arch | Platform | Repository URL | Static metadata |
|----------|----------|----------------|-----------------|
${archRows}

**Signing key**: \`${keyUrl}\`

**Fingerprint**: \`${apkRepository.keyFingerprint}\`

Verify the fingerprint before installing the key or any package.

## Install Instructions

Do not enable this repository on production systems yet. For disposable test containers after repository verification:

\`\`\`sh
set -eu

apk_arch="$(apk --print-arch)"
repo_url="${repoRoot}"

wget -O "/etc/apk/keys/${apkRepository.keyFile}" "${keyUrl}"
printf '%s\n' "$repo_url" >> /etc/apk/repositories
apk update

# Example, once packages are published:
# apk add <verity-package>
\`\`\`

If your image lacks \`wget\`, copy the key into \`/etc/apk/keys/${apkRepository.keyFile}\` during image build and append only the matching architecture URL to \`/etc/apk/repositories\`.

## Trust Model

- APK metadata is published as signed \`APKINDEX.tar.gz\` files per architecture.
- APK clients trust packages through the public key installed in \`/etc/apk/keys/\`.
- Verify the key fingerprint from this page before installing any package.
- Keep Verity container-image signatures and attestations separate from APK repository trust; cosign/SLSA cover OCI images, while APK installation relies on APKINDEX/package signatures.

## Key Rotation

1. Verity will publish a new key and fingerprint before rotating signing keys.
2. Install both old and new keys during the overlap window.
3. Run \`apk update\` and verify the signed index refreshes cleanly.
4. Remove the retired key only after the repository announces completion of the rotation.

## Experimental Caveats

- Package names, versions, repository paths, and signing keys may change before general availability.
- The repository is rolling and may remove packages no longer referenced by the current index.
- Use only in ephemeral tests until final availability is verified.
- Prefer the published container images for production workloads today.

---

[Browse Catalog](${prefix}) · [Complete LLM Reference](${prefix}llms-full.txt) · [GitHub](https://github.com/verity-org/verity)
`;

  return new Response(`${content.trim()}\n`, {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
