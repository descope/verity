# APK Signing Key Specification

## Decision

Verity MUST use a stable **RSA-4096** key with public exponent **65537** and
APK signature type **RSA256** (RSA PKCS#1 v1.5 with SHA-256) for production APK
packages and `APKINDEX.tar.gz` files.

ECDSA and GitHub OIDC/Sigstore signatures MUST NOT replace native APK
signatures. OIDC attestations are an additional provenance layer.

## Cryptographic profile

| Property | Requirement |
| --- | --- |
| Algorithm | RSA |
| Modulus | 4096 bits |
| Public exponent | 65537 |
| APK signature type | `RSA256` |
| Digest | SHA-256 |
| Private-key encoding | PEM PKCS#8 |
| Public-key encoding | PEM SubjectPublicKeyInfo |
| Public-key suffix | `.rsa.pub` |
| Fingerprint | SHA-256 of DER-encoded SubjectPublicKeyInfo |

`abuild-sign` defaults to signature type `RSA`, which means RSA with SHA-1.
Production automation MUST therefore pass `-t RSA256` explicitly. SHA-1,
RSA keys below 3072 bits, DSA, unsigned indexes, and `--allow-untrusted` are
forbidden.

## Key custody

- The private PEM is stored as the protected GitHub Actions environment secret
  `APK_REPOSITORY_PRIVATE_KEY`; it is never committed or uploaded as an artifact.
- Only a protected production signing environment on protected `main` may read
  the secret. Manual dispatch must not select an arbitrary ref.
- `ci/apk-signing-key-state.json` is the canonical public trust-state input.
  Go validation requires its active fingerprint to match the committed canonical
  SPKI key before publication composition.
- GitHub injects the secret only into the exact signing step. That step copies it
  to a local non-exported variable, immediately unsets the environment entry,
  and passes it to the Go signer through standard input. Workflow/job/container
  environments, command arguments, Docker metadata, logs, and xtrace must not
  carry the PEM.
- The Go signer removes the legacy ambient variable before spawning children.
  Child processes receive an allowlisted environment and never inherit the key.
- The PEM may be materialized only after immutable-image attestation succeeds,
  inside a signer-only memory-backed directory with mode `0600`. Every success,
  failure, cancellation, and panic path zeroizes in-memory buffers and removes
  the materialized key before returning.
- The signer image is addressed by an immutable SHA-256 digest and its provenance
  attestation is mandatory. The key-bearing container is offline, non-root,
  read-only, capability-free, and unable to gain privileges. It receives only
  narrow data mounts; mounting the whole workspace read-write is forbidden.
- Runtime package installation, host-resolved signing executables, mutable image
  tags, and local `go build`/`go run` compilation are forbidden in the key
  boundary.
- Pull-request and untrusted workflows MUST never receive the production key.
- GitHub OIDC provenance attestations MUST be generated for every published
  APK and verified before repository assembly.

## Signing requirements

The release path MUST re-sign approved packages with the stable production key
after verifying their GitHub OIDC provenance. Packages and indexes use the same
key to provide one unambiguous client trust root.

Melange package signatures and repository indexes MUST contain signature entries
named `.SIGN.RSA256.<public-key-filename>`.

Index signing command:

```sh
abuild-sign -t RSA256 -k "$APK_SIGNING_KEY" APKINDEX.tar.gz
```

## Generation and fingerprint

Generate the key inside its final custody boundary:

```sh
openssl genpkey -algorithm RSA \
  -pkeyopt rsa_keygen_bits:4096 \
  -pkeyopt rsa_keygen_pubexp:65537 \
  -out verity.rsa

openssl pkey -in verity.rsa \
  -pubout -out verity.rsa.pub

openssl pkey -pubin -in verity.rsa.pub \
  -outform DER | sha256sum
```

The current lowercase hexadecimal fingerprint is
`416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7`.

## Mandatory validation gate

Publication MUST fail unless all checks pass for `x86_64` and `aarch64`:

1. Verify each APK's GitHub provenance attestation and expected workflow identity.
2. Cryptographically verify every APK with the published key using `apk verify`.
3. Verify each `APKINDEX.tar.gz` contains an `RSA256` signature for that key.
4. In a fresh pinned Alpine container, install the key and run `apk update`
   without `--allow-untrusted` for every published architecture.
5. Repeat package verification with a wrong key and require it to fail.
6. Publish only allowlisted APKs whose associated Integer build passed the
   strict zero-CVE gate.
7. Before exposing the signing key, reject malformed or ambiguous APKs,
   traversal paths, symlinks, hard links, path swaps, trailing data, and
   compressed metadata/data that exceed the bounded archive limits.
8. Require the exact private/public RSA pair and reject signature or content
   tampering after signing.

Checking only that a `.SIGN.*` member exists is insufficient.

An intentional change to index-generation or signing semantics MUST increment
the published `repository-format` marker. Otherwise an unchanged signed APK and
key set deliberately preserves the previously published index bytes.

## Rotation, revocation, and rollback fencing

- Every publication manifest carries a monotonically increasing signing-key
  epoch, one active fingerprint, the bounded trusted overlap set, and an
  explicit revoked set. A candidate epoch lower than the previously authenticated
  Pages manifest is rejected, including restore operations.
- The genesis state is epoch `1`, trusts only the current fingerprint, and records
  the unpublished predecessor fingerprint as revoked so rollback cannot activate it.
- Rotate routinely every 12 months and immediately after suspected compromise.
  Generate the replacement inside its approved custody boundary; never retrieve
  the old private key for migration.
- For routine rotation, publish the new fingerprinted public key at least 30 days
  before activation. During the bounded overlap, clients trust both fingerprints,
  while new packages and indexes use only the active key.
- Retirement removes the old fingerprint from the trusted overlap set only after
  all artifacts signed by it are gone and the client migration window has closed.
- Revocation is immediate and overrides overlap or retirement schedules. The
  active fingerprint must never also appear in the revoked set, and rollback may
  not resurrect a revoked or lower-epoch key.

## Incident response

If exclusive custody may have been lost, stop APK publication, preserve workflow
run and artifact metadata, and treat the key as exposed. Revoke the fingerprint,
advance the key epoch, create a replacement under approved custody, invalidate
affected artifacts, audit every published package and index, and rebuild only
from protected `main` with the attested zero-CVE signer. Do not bypass Trivy,
signature, provenance, or client verification gates to restore service.

## Primary references

- Alpine `abuild-sign`: https://github.com/alpinelinux/abuild/blob/master/abuild-sign.in
- Alpine `abuild-keygen`: https://github.com/alpinelinux/abuild/blob/master/abuild-keygen.in
- APK v2 signature format: https://github.com/alpinelinux/apk-tools/blob/master/doc/apk-v2.5.scd
- NIST SP 800-57 Part 1 Rev. 5: https://csrc.nist.gov/pubs/sp/800/57/pt1/r5/final
