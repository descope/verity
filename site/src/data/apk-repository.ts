export const apkRepository = {
  status: "experimental",
  basePath: "apk",
  architectures: [
    { apk: "x86_64", platform: "linux/amd64" },
    { apk: "aarch64", platform: "linux/arm64" },
  ],
  keyFile: "verity.rsa.pub",
  // biome-ignore lint/security/noSecrets: This is the published public-key fingerprint.
  keyFingerprint: "90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2",
  caveat:
    "The Verity APK repository is experimental. Every published package is provenance-verified, re-signed with the stable Verity RSA256 key, and admitted only after its Integer build passes the zero-CVE gate.",
};
