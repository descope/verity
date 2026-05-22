# Guide — Install from Verity's experimental APK repository

Verity's APK repository is experimental. It is intended for early adopters who
want to install Verity-built APKs directly on Alpine-compatible systems. For
production workloads, prefer Verity's signed container images until the APK
channel graduates from experimental status.

## Repository URLs

| Architecture | Repository URL |
| --- | --- |
| `x86_64` | `https://verity.supply/apk/x86_64` |
| `aarch64` | `https://verity.supply/apk/aarch64` |

Public key: `https://verity.supply/apk/verity.rsa.pub`

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

printf '%s\n' "https://verity.supply/apk/$arch" >> /etc/apk/repositories

apk update
```

Then install a package:

```sh
apk add <package-name>
```

## Verify repository state

After implementation creates the production public key, compare the documented
SHA-256 fingerprint with the installed key:

```sh
sha256sum /etc/apk/keys/verity.rsa.pub
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

## Remove the repository

Edit `/etc/apk/repositories` and remove the matching
`https://verity.supply/apk/<arch>` line. Then remove the key if no other Verity
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
