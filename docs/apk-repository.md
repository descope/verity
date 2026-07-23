# Experimental APK repository publishing

Verity can assemble Melange `.apk` artifacts into a static APK repository under
`site/dist/apk/` for GitHub Pages publishing.

## Artifact sources

Integer image builds upload `apk-repository-<batch>-*` artifacts only after the
strict zero-CVE build and registry scan pass. Each APK receives a GitHub OIDC
build-provenance attestation. The Pages workflow downloads artifacts only from
the exact successful scheduled Integer run selected by `wait-for-workflows.sh`,
then verifies every attestation against the reusable Integer builder identity
before any signing secret is exposed.

## Signing

`APK_REPOSITORY_PRIVATE_KEY` is a protected `github-pages` environment secret.
The matching public key is committed at `keys/apk/verity.rsa.pub` and published
byte-for-byte at `https://verity.supply/apk/verity.rsa.pub`. Publication fails if
the private key does not match the committed key.

Approved packages are re-signed with Melange using RSA/SHA-256, and indexes are
signed explicitly with `abuild-sign -t RSA256`.

## Guarded empty behavior

Local unsigned assembly may write `site/dist/apk/.no-apks-found`. Protected Pages
publication fails earlier if the trusted Integer run produced no approved APKs.

## Local validation

Non-empty repository assembly requires Alpine `apk` tooling. Signed assembly also
requires `abuild-sign` and `openssl`. On non-Alpine hosts, run the same pinned
container used by CI:

```bash
docker run --rm \
  -e APK_REPOSITORY_PRIVATE_KEY \
  -v "$PWD:/work" \
  -v "$(command -v melange):/usr/local/bin/melange:ro" \
  -w /work \
  alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601 \
  sh -euxc 'apk add --no-cache abuild bash findutils openssl; bash .github/scripts/assemble-apk-repository.sh --output site/dist/apk packages/repo apk-artifacts'

bash .github/scripts/validate-apk-repository.sh site/dist/apk
```

Run `validate-apk-repository.sh --verify-crypto` inside the same Alpine image for
the publication-grade package and fresh-client index checks.
