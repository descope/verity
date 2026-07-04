# Remediation Policy — Copa-first

## Invariant

**Verity never publishes an image with a HIGH/CRITICAL CVE.**

This is enforced in CI, not aspirational: the Integer build gates publish on a
local Trivy scan of every arch (`verity integer build --fail-on-severity
HIGH,CRITICAL`, wired into `integer-build-image.yaml`). Not clean → no publish.
If an image cannot be made clean, it is **withheld** and a tracking issue is
opened instead.

## Remediation ordering

When an image has fixable CVEs, remediate in this order:

1. **Copa-patch the upstream image — the default, first choice.**
   In-place OS/app-package patching of the *real upstream image* (see the
   Discover → Scan → Patch → Sign → Publish pipeline in the README). Preferred
   because:
   - **Maximum compatibility** — it's the vendor's actual image, patched, so
     runtime behavior, layout, entrypoints, and integrations match upstream.
   - Copa can apply **distro-patched packages** that a from-scratch Wolfi
     rebuild often cannot (the fix exists upstream even when Wolfi's APK is
     stale).

2. **Wolfi / bespoke Integer rebuild — last resort.**
   A from-scratch hardened rebuild from Wolfi packages (+ bespoke melange
   source builds). Use **only** when:
   - Copa cannot reach zero HIGH/CRITICAL (no upstream package fix available,
     statically-linked vulnerable libraries, distroless base Copa can't patch),
     **or**
   - a minimal-attack-surface / FIPS hardened variant is explicitly required.

## Consequence

- If **Copa** clears the CVEs → ship the Copa-patched variant (the supported
  path). Any CVE-laden Integer variant of the same image is withheld by the
  gate.
- If **neither** Copa nor a bespoke rebuild can clear the CVEs → the image is
  **not published**; a tracking issue stays open until upstream (or Wolfi)
  ships a fix. This is honest exposure, not a silent vulnerable release.

## Note

This **supersedes** the earlier guidance in `copa-config.yaml` that treated
Wolfi rebuilds as the default and Copa as the fallback for "images that cannot
be trivially rebuilt from Wolfi." The ordering is now the reverse: Copa-first
for compatibility and breadth of coverage, Wolfi/bespoke as the deliberate
last resort.
