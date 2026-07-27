export const apkRepository = {
  status: "experimental",
  basePath: "apk",
  architectures: [
    { apk: "x86_64", platform: "linux/amd64" },
    { apk: "aarch64", platform: "linux/arm64" },
  ],
  keyFile: "verity.rsa.pub",
  // biome-ignore lint/security/noSecrets: This is the published public-key fingerprint.
  keyFingerprint: "416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7",
  caveat:
    "The Verity APK repository is experimental. Every published package is provenance-verified, re-signed with the stable Verity RSA256 key, and admitted only after its Integer build passes the zero-CVE gate.",
};
