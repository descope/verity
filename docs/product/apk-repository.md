# Product specification — Experimental APK repository

Verity will publish an experimental, signed APK repository alongside the public
site at `https://verity.supply/apk/`. The repository gives Alpine-compatible
clients a direct package-install path for Verity-built artifacts while the
primary production surfaces remain container images, Integer images, Helm
wrapper charts, and the web catalog.

## Product promise

- **Experimental channel:** suitable for early adopters and integration testing;
  not yet a stable production SLA.
- **Rolling latest only:** clients receive the latest indexed package set for
  their architecture. There are no date snapshots or stable/edge channels in the
  MVP.
- **Signed repository metadata:** clients must trust the Verity APK public key
  and verify its published SHA-256 fingerprint before installing packages.
- **Static delivery:** repository files are served by GitHub Pages under the
  same domain as the catalog.

## MVP scope

| Area | MVP decision |
| --- | --- |
| Base URL | `https://verity.supply/apk/` |
| Architectures | `x86_64`, `aarch64` |
| Channels | Rolling `latest` only; no explicit channel segment |
| Public key | `https://verity.supply/apk/verity.rsa.pub` current-key alias; fingerprinted key files may be published during rotation |
| Client repository URL | `https://verity.supply/apk` (`apk` appends the architecture) |
| Index | `/apk/<arch>/APKINDEX.tar.gz`, signed with Verity APK signing key |
| Packages | Current Verity-built `.apk` files referenced by the index |

## User experience

The user flow is intentionally small:

1. Download and install the Verity APK public key into `/etc/apk/keys/`.
2. Add the matching architecture repository URL to `/etc/apk/repositories`.
3. Run `apk update`.
4. Install a package with `apk add <package>`.
5. Verify package and repository state with `apk policy`, `apk info`, and the
   documented key fingerprint.

The stable install guide lives at
[`docs/guides/install-apk-repository.md`](../guides/install-apk-repository.md).

## Retention and compatibility

The repository retains only packages referenced by the current architecture
index. Superseded packages may disappear without notice while the channel is
experimental. Consumers that need reproducibility should pin container image
digests or wait for a future versioned APK channel SCR.

## Graduation signals

The experimental label can be revisited after Verity has:

- passed install smoke tests for both MVP architectures,
- exercised one signing-key rotation drill or documented tabletop,
- demonstrated cleanup of superseded packages without stale index references,
- published package provenance and verification guidance on the public site, and
- created follow-up support policy for breakage reporting.
