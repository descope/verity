# Experimental APK repository publishing

Verity can assemble Melange `.apk` artifacts into a static APK repository under
`site/dist/apk/` for GitHub Pages publishing.

## Artifact sources

Integer image builds already create Melange package artifacts in
`.github/workflows/integer-build-image.yaml`:

- `melange-packages-x86_64`
- `melange-packages-aarch64`

Each artifact contains a `packages/repo/` tree with architecture-specific
subdirectories. The repository assembly scripts scan `packages/repo/` and
`apk-artifacts/` for those layouts.

The privileged Pages workflow does **not** download artifacts from arbitrary
workflow runs. Doing so would allow artifact poisoning if a caller selected an
untrusted run. A follow-up should add a trusted handoff (for example, a same-run
build-and-assemble job chain, provenance checks, or a protected package storage
decision) before cross-run artifacts are published automatically.

## Signing

Set `APK_REPOSITORY_PRIVATE_KEY` as a GitHub Actions secret containing the PEM
private key used to sign `APKINDEX.tar.gz`. The workflow derives and publishes
`verity-apk-repository.rsa.pub` next to the repository root.

If the secret is absent, the workflow still assembles unsigned indexes and runs
non-signature validation. This keeps repository scaffolding safe for branches and
forks while making signed publishing available on protected environments.

## Guarded empty behavior

If no `.apk` artifacts are available, assembly writes `site/dist/apk/.no-apks-found`
and exits successfully. This prevents scheduled site deploys from failing while
the package retention and repository-size policy is still being decided.

## Local validation

Non-empty repository assembly requires Alpine `apk` tooling. Signed assembly also
requires `abuild-sign` and `openssl`. On non-Alpine hosts, run the same pinned
container used by CI:

```bash
docker run --rm \
  -e APK_REPOSITORY_PRIVATE_KEY \
  -v "$PWD:/work" \
  -w /work \
  alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601 \
  sh -euxc 'packages="bash findutils"; if [ -n "${APK_REPOSITORY_PRIVATE_KEY:-}" ]; then packages="$packages abuild openssl"; fi; apk add --no-cache $packages; bash .github/scripts/assemble-apk-repository.sh --output site/dist/apk packages/repo apk-artifacts'

bash .github/scripts/validate-apk-repository.sh site/dist/apk
```

Use `--require-signature` once `APK_REPOSITORY_PRIVATE_KEY` is configured.
