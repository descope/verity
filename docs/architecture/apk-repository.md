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
3. Validate package structure, paths, sizes, semantic identity, and the exact
   active RSA-4096/65537 public key before exposing private material.
4. Pull and attest the immutable signer image, then pass the production key to
   the Go signer through stdin. Materialize it only in the isolated signer
   boundary and re-sign candidate packages and indexes offline.
5. Restore the latest non-expired main-branch Pages artifact and verify its APK
   signatures and indexes independently.
6. Compare signed APK paths and digests, public-key digests, and the explicit
   `repository-format` marker.
7. Preserve the previous APK tree byte-for-byte when that state is unchanged;
   otherwise select the complete candidate.
8. Verify the selected repository with fresh Alpine clients for `x86_64` and
   `aarch64`, then upload the complete site as the next Pages artifact.

The publish job must fail closed if any required architecture is missing, any
index is unsigned, an APK signature fails verification, or a fresh client cannot
load an architecture index.

## Signing and key custody

- The private signing key is stored as a GitHub Actions secret scoped to the
  autonomous `apk-signing` environment and exact `main` deployments.
- The current public key is non-secret and published as `/apk/verity.rsa.pub`.
- Documentation and site copy show the SHA-256 SPKI fingerprint
  `416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7`
  next to the public-key URL.
- The protected workflow reads the private key only on protected `main`. GitHub
  injects it into the exact signing step, which immediately copies it to a local
  non-exported variable, unsets the environment entry, and sends it to the Go
  signer on stdin. Workflow/job/container environments, argv, xtrace, artifacts,
  caches, and inherited child environments must not carry the secret.
- The immutable signer digest is attested before key materialization. Key-bearing
  execution is offline, non-root, read-only, capability-free, and limited to
  narrow read-only inputs plus one output mount. Runtime installation, mutable
  executables, and writable whole-workspace mounts are forbidden.
- Key material lives only in a signer-private memory-backed directory, is mode
  `0600`, and is zeroized and removed on success, failure, or cancellation.
- Key rotation is additive first: publish a fingerprinted new public key,
  announce the new fingerprint, ask clients to install both old and new keys,
  move the `/apk/verity.rsa.pub` alias to the new key for future installs, sign
  future indexes with the new key, then remove retired-key instructions after
  the overlap period.

## Rotation and rollback state

The authenticated publication manifest records a monotonically increasing key
epoch, the active fingerprint, the bounded trusted overlap set, and revoked
fingerprints. Routine rotation advances the epoch and temporarily trusts old and
new keys. Retirement removes the old key after all old signatures and client
instructions expire. Revocation removes trust immediately. Snapshot, delta, and
restore paths reject lower epochs and any state that makes a revoked key active.
The repository-owned `ci/apk-signing-key-state.json` binds that state to the
canonical RSA public key before a manifest can be composed.

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

Suspected key exposure is a publication stop, not a normal generation failure.
Preserve run/artifact evidence, revoke the old fingerprint, advance the epoch,
replace the key without retrieving the old secret, invalidate affected artifacts,
audit published signatures, and rebuild from protected `main` with the attested
signer and mandatory all-severity Trivy gate.
