# Technical architecture — Experimental APK repository

This document is the steady-state contract for Verity's experimental APK
repository. It complements the higher-level system map in
[`ARCHITECTURE.md`](../../ARCHITECTURE.md) and the product contract in
[`docs/product/apk-repository.md`](../product/apk-repository.md).

## Repository layout

The MVP repository is static and published through GitHub Pages:

```text
https://verity.supply/apk/
├── verity.rsa.pub               # current-key alias
├── verity-<fingerprint>.rsa.pub # optional rotation/overlap key
├── repository-format            # publication semantics version
├── x86_64/
│   ├── APKINDEX.tar.gz
│   └── *.apk
└── aarch64/
    ├── APKINDEX.tar.gz
    └── *.apk
```

Rules:

- `x86_64` and `aarch64` are the only required MVP architecture directories.
- Each architecture directory is self-contained: the index references only APKs
  in that directory.
- `APKINDEX.tar.gz` is always signed. An unsigned index must not be uploaded.
- The current public key is available at `/apk/verity.rsa.pub`. During rotation,
  fingerprinted key files such as `/apk/verity-<fingerprint>.rsa.pub` may also
  be published so clients can install both old and new trust anchors before the
  alias moves.
- No package outside the current index is part of the repository contract.

## Build and publish flow

The intended implementation sequence is:

1. Build a complete candidate from the exact successful scheduled Integer batch.
2. Verify every candidate artifact's GitHub provenance before exposing the
   repository signing secret.
3. Re-sign candidate packages, generate per-architecture indexes, and sign the
   indexes with the stable APK key.
4. Restore the latest non-expired main-branch Pages artifact and verify its APK
   signatures and indexes independently.
5. Compare signed APK paths and digests, public-key digests, and the explicit
   `repository-format` marker.
6. Preserve the previous APK tree byte-for-byte when that state is unchanged;
   otherwise select the complete candidate.
7. Verify the selected repository with fresh Alpine clients for `x86_64` and
   `aarch64`, then upload the complete site as the next Pages artifact.

The publish job must fail closed if any required architecture is missing, any
index is unsigned, an APK signature fails verification, or a fresh client cannot
load an architecture index.

## Signing and key custody

- The private signing key is stored as a GitHub Actions secret scoped to the
  Pages/repository publish workflow.
- The current public key is non-secret and published as `/apk/verity.rsa.pub`.
- Documentation and site copy show the SHA-256 SPKI fingerprint
  `90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2`
  next to the public-key URL.
- CI logs must not print private key material. Workflows should write private
  material to temporary files with restrictive permissions and delete them after
  signing.
- Key rotation is additive first: publish a fingerprinted new public key,
  announce the new fingerprint, ask clients to install both old and new keys,
  move the `/apk/verity.rsa.pub` alias to the new key for future installs, sign
  future indexes with the new key, then remove retired-key instructions after
  the overlap period.

## Retention policy

This repository is rolling `latest` only. The Pages artifact should contain:

- the current public key,
- one signed index per MVP architecture,
- APKs referenced by those indexes.

Superseded APKs should be removed when no current `APKINDEX.tar.gz` references
them. GitHub Actions artifacts, logs, and temporary staging directories are
implementation details and do not create a supported package archive.

The `github-pages` workflow artifact is retained for 30 days solely as
authenticated previous state. It prevents unchanged nightly builds from
re-signing indexes. It is not a historical package channel.

## Verification contract

Before publishing or advertising a repository update, automation should verify:

1. `/apk/verity.rsa.pub` exists and matches the documented
   fingerprint.
2. Each `APKINDEX.tar.gz` includes an APK signature entry accepted by `apk` when
   the Verity public key is installed.
3. Every package referenced by an index exists in the same architecture
   directory.
4. A fresh Alpine-compatible client can run `apk update` against each
   architecture URL.
5. Every published package passes `apk verify` with the committed public key.

## Failure handling

If repository generation fails, the publish workflow should leave the previously
published Pages artifact in place rather than upload a partial `/apk/` tree. If a
bad repository is published, remediation is: regenerate from the last known-good
package set, publish a fixed signed index, then file a follow-up issue with root
cause and prevention steps.
