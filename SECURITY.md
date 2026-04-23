# Security

## Reporting Security Vulnerabilities

**Do not open public GitHub issues for security vulnerabilities.**

Report vulnerabilities privately through [GitHub Security Advisories](https://github.com/verity-org/verity/security/advisories/new).
You'll receive an acknowledgment within 48-72 hours. We aim to release a fix
or mitigation within 14 days for critical issues, 30 days for others.

Include in your report:

- Affected component (CLI, workflow, published image)
- Steps to reproduce
- Potential impact
- Any suggested remediation

## Supported Versions

Only the latest release on the `main` branch receives security patches.
Older releases are not backported. If you're running a pinned version,
upgrade to the latest patched image tag.

## Security Model

Verity's security posture is built around supply chain integrity, not just
vulnerability patching.

**Signing and attestation.** Every patched image is signed using cosign with
keyless OIDC via Sigstore. No long-lived signing keys exist. Signatures are
tied to the specific GitHub Actions workflow run that produced them, and
verified against Fulcio's certificate transparency log.

**SBOM attestation.** An SBOM is attached as an in-toto attestation via
[`actions/attest`](https://github.com/actions/attest) alongside each image.
Copa-patched images ship a CycloneDX SBOM produced by Syft after patching;
Integer images ship an SPDX SBOM produced by apko during the from-scratch
build. Consumers can inspect the full package inventory without trusting
Verity's infrastructure directly.

**Scanning.** Trivy scans every image before patching to identify fixable
CVEs. Only images with actionable vulnerabilities proceed through the pipeline.
Scan reports (pre- and post-patch) are pushed to the `reports` branch for
auditability, but are not currently attached to images as in-toto attestations.

**Patching.** Copa patches OS-level packages (`apt`, `yum`/`dnf`, `apk`),
Python packages (`pip`), and Go binaries (via `goVcsUrl` declaring the upstream
module VCS path) in-place without rebuilding from a Dockerfile. This
eliminates the risk of accidental dependency changes during a rebuild.

## Verifying Images

All patched images are verifiable without trusting Verity's registry directly.

```bash
# Verify the cosign signature
cosign verify \
  --certificate-identity-regexp "https://github.com/verity-org/verity/.github/workflows/" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched

# Verify the SBOM attestation (GitHub CLI)
gh attestation verify \
  oci://ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched \
  --owner verity-org --predicate-type https://cyclonedx.org/bom

# Inspect the CycloneDX SBOM directly
cosign download attestation \
  ghcr.io/verity-org/prometheus/prometheus:v3.9.1-patched \
  | jq '.payload | @base64d | fromjson | select(.predicateType | contains("cyclonedx"))'
```

A valid signature confirms the image was built by Verity's CI pipeline, not
modified after publication. The cosign signing certificate records the workflow
identity (repository + workflow path) that produced it and the corresponding
Rekor entry binds it to a specific build.

## Supply Chain Security

**Pinned action versions.** All GitHub Actions are pinned to full SHA digests,
not mutable tags. Renovate automatically opens PRs to update pins when new
releases are available.

**Hardened runners.** Workflows use `step-security/harden-runner` to audit
egress traffic and block unexpected network access during CI.

**Dependency updates.** Renovate monitors Go modules, GitHub Actions, Docker
base images, and Helm chart versions. Updates arrive as automated PRs with
changelogs. See [.github/RENOVATE.md](.github/RENOVATE.md) for the
configuration.

**Minimal permissions.** Workflows declare explicit `permissions` blocks.
The patch pipeline requests `packages: write` and `id-token: write` only for
the signing step. All other jobs default to read-only.

## Code Security Practices

**Static analysis.** `golangci-lint` runs on every PR with `gosec` enabled,
flagging common Go security issues (hardcoded credentials, unsafe file
operations, unhandled errors on security-relevant calls).

**Vulnerability scanning.** `govulncheck` scans Go module dependencies against
the Go vulnerability database on every PR and in the daily pipeline.

**Secret detection.** `gitleaks` runs as a pre-commit hook and in CI to catch
accidentally committed credentials before they land in the repository.

**Test coverage.** The pipeline enforces a minimum 80% code coverage threshold.
All tests run with `-race` to surface data race conditions.

## Compliance

Verity's pipeline runs on GitHub-hosted infrastructure with isolated build
environments per job, keyless cosign signing, and SBOM attestations — covering
a meaningful subset of SLSA's source/build integrity practices. A formal
SLSA build-provenance attestation is not emitted today.

Detailed mappings to FedRAMP, SOC 2, ISO 27001, NIST SP 800-53, and OWASP
controls are available at
[verity.supply/compliance](https://verity.supply/compliance/).

## Security Contacts

Use [GitHub Security Advisories](https://github.com/verity-org/verity/security/advisories)
for all security reports. This is the only channel monitored for vulnerability
disclosures.
