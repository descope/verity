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

`APK_REPOSITORY_PRIVATE_KEY` is an `apk-signing` environment secret restricted
to deployments from `main`. The environment has no reviewer or wait-timer gate,
so protected `main` publication remains autonomous.
The matching public key is committed at `keys/apk/verity.rsa.pub` and published
byte-for-byte at `https://verity.supply/apk/verity.rsa.pub`. Publication fails if
the private key does not match the committed key.

Approved packages are re-signed with Melange using RSA/SHA-256, and indexes are
signed explicitly with `abuild-sign -t RSA256`.

## Update discipline

Every scheduled publication builds a complete candidate repository from one
successful Integer batch. It also restores the latest retained main-branch Pages
artifact and cryptographically verifies both repositories.

A merged recipe change reaches the APK repository through the next successful
scheduled Integer batch. Failed or partial batches never mutate the published
repository.

Publication compares the path and SHA-256 digest of every signed APK, public key,
and `repository-format` marker:

- If the state is identical, the previously published APKs and indexes are
  copied byte-for-byte. A nightly rebuild must not create repository churn.
- If package contents, package paths, the trust root, or the repository format
  change, the complete candidate replaces the previous repository.
- Recipe removal is therefore handled safely: the removed package is absent
  from the complete candidate and disappears from the rolling repository.
- Site-only and workflow-only changes do not reissue APK indexes unless they
  intentionally bump `repository-format`.

The Pages artifact is retained for 30 days so daily runs have authenticated
previous state. A missing prior artifact is a first-publication bootstrap, not
permission to merge a partial package set.

## Guarded empty behavior

Local unsigned assembly may write `site/dist/apk/.no-apks-found`. Protected Pages
publication fails earlier if the trusted Integer run produced no approved APKs.

## Local validation

Non-empty repository assembly requires Alpine `apk` tooling. Signed assembly also
invokes Melange and `abuild-sign`; key parsing, key matching, archive validation,
and publication policy remain inside the Verity Go CLI. On non-Alpine hosts, run
the same pinned container used by CI:

```bash
CGO_ENABLED=0 go build -o verity .
docker run --rm \
  -e APK_REPOSITORY_PRIVATE_KEY \
  -v "$PWD:/work" \
  -v "$(command -v melange):/usr/local/bin/melange:ro" \
  -w /work \
  alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601 \
  sh -euxc 'apk add --no-cache abuild gcompat; /work/verity ci apk-repository assemble --output site/dist/apk packages/repo apk-artifacts'

./verity ci apk-repository validate site/dist/apk
```

Run `verity ci apk-repository validate --verify-crypto` inside the same Alpine
image for the publication-grade package and fresh-client index checks.
