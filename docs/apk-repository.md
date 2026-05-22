# Experimental APK repository publishing

Verity can assemble Melange `.apk` artifacts into a static APK repository under
`site/dist/apk/` for GitHub Pages publishing.

## Artifact sources

Integer image builds already create Melange package artifacts in
`.github/workflows/integer-build-image.yaml`:

- `melange-packages-x86_64`
- `melange-packages-aarch64`

Each artifact contains a `packages/repo/` tree with architecture-specific
subdirectories. The APK repository workflow downloads those artifacts from a
caller-provided workflow run id and indexes any `.apk` files it finds.

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

```bash
bash .github/scripts/assemble-apk-repository.sh --output site/dist/apk packages/repo apk-artifacts
bash .github/scripts/validate-apk-repository.sh site/dist/apk
```

Use `--require-signature` once `APK_REPOSITORY_PRIVATE_KEY` is configured.
