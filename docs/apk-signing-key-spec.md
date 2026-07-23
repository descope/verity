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
- Only the `github-pages` environment publication jobs receive the secret.
- The PEM may exist only in an ephemeral runner temporary directory with mode
  `0600`; it is deleted when the assembly process exits.
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
`90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2`.

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

Checking only that a `.SIGN.*` member exists is insufficient.

An intentional change to index-generation or signing semantics MUST increment
the published `repository-format` marker. Otherwise an unchanged signed APK and
key set deliberately preserves the previously published index bytes.

## Rotation

- Rotate routinely every 12 months and immediately after suspected compromise.
- Publish the new public key and fingerprint at least 30 days before switching.
- During overlap, clients MUST trust both old and new keys.
- Keep the old public key available until every package and index signed by it
  has expired from the repository.
- A compromised key MUST be removed from publishing immediately; rebuilding and
  republishing affected packages is mandatory.

## Primary references

- Alpine `abuild-sign`: https://github.com/alpinelinux/abuild/blob/master/abuild-sign.in
- Alpine `abuild-keygen`: https://github.com/alpinelinux/abuild/blob/master/abuild-keygen.in
- APK v2 signature format: https://github.com/alpinelinux/apk-tools/blob/master/doc/apk-v2.5.scd
- NIST SP 800-57 Part 1 Rev. 5: https://csrc.nist.gov/pubs/sp/800/57/pt1/r5/final
