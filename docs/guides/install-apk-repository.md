# Guide — Install from Verity's experimental APK repository

Verity's APK repository is experimental. It is intended for early adopters who
want to install Verity-built APKs directly on Alpine-compatible systems. For
production workloads, prefer Verity's signed container images until the APK
channel graduates from experimental status.

## Repository URL

Configure this base URL; `apk` automatically appends the client architecture:

`https://verity.supply/apk`

The resulting static endpoints are:

| Architecture | Index endpoint |
| --- | --- |
| `x86_64` | `https://verity.supply/apk/x86_64` |
| `aarch64` | `https://verity.supply/apk/aarch64` |

Public key: `https://verity.supply/apk/verity.rsa.pub`

SHA-256 fingerprint of the DER-encoded public key:
`416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7`

## Install

Run these commands as root inside the Alpine-compatible system:

```sh
arch="$(apk --print-arch)"

case "$arch" in
  x86_64|aarch64) ;;
  *) echo "unsupported Verity APK repository architecture: $arch" >&2; exit 1 ;;
esac

wget -O /etc/apk/keys/verity.rsa.pub \
  https://verity.supply/apk/verity.rsa.pub

repo="https://verity.supply/apk"
if ! grep -qxF "$repo" /etc/apk/repositories; then
  printf '%s\n' "$repo" >> /etc/apk/repositories
fi

apk update
```

Then install a package:

```sh
apk add <package-name>
```

## Verify repository state

Compare the installed key with the fingerprint above:

```sh
openssl pkey -pubin -in /etc/apk/keys/verity.rsa.pub -outform DER | sha256sum
```

Confirm `apk` sees the Verity repository and package source:

```sh
apk update
apk policy <package-name>
apk info -vv <package-name>
```

`apk update` must fail if the signed index cannot be verified with the installed
Verity public key. Treat a signature failure as a stop condition: do not bypass
signature verification for this repository.

## Key rotation and revocation

During an announced rotation window, install each explicitly published
fingerprinted key in `/etc/apk/keys` and verify every fingerprint against this
guide or the signed release notice before running `apk update`. Keep both keys
only for the stated overlap period. The `/apk/verity.rsa.pub` alias identifies
the key for new installations; it does not authorize an unannounced fingerprint
change.

If Verity announces a revocation, remove the revoked fingerprinted key
immediately, install the replacement key, verify its fingerprint, and run
`apk update`. Do not restore an older key or repository snapshot to work around
signature failures: publication key epochs are monotonic, and rollback must not
resurrect retired or revoked trust.

If the downloaded key fingerprint differs from the documented or announced
value, stop. Preserve the key and command output for incident reporting, remove
the repository URL until the discrepancy is resolved, and do not use
`--allow-untrusted`.

## Remove the repository

Edit `/etc/apk/repositories` and remove the matching
`https://verity.supply/apk` line. Then remove the key if no other Verity
APK repository uses it:

```sh
rm -f /etc/apk/keys/verity.rsa.pub
apk update
```

## Support boundaries

The MVP repository is rolling `latest` only. Old APK files may be removed when
they are no longer referenced by the current index. If you need reproducible
rollbacks, pin a Verity container image digest or wait for a future versioned APK
repository channel.
