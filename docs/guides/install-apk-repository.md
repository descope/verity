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

SHA-256 fingerprint: `TBD` until the implementation publishes the production
key. Do not use the repository until this value is replaced by a real
fingerprint on `verity.supply` and in this guide.

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

repo="https://verity.supply/apk/$arch"
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

Before trusting the repository, confirm this guide or `verity.supply` publishes
a non-`TBD` SHA-256 fingerprint for the production key. Then compare that value
with the installed key:

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
