export const apkRepository = {
  status: "experimental pending availability",
  basePath: "apk",
  architectures: [
    { apk: "x86_64", platform: "linux/amd64" },
    { apk: "aarch64", platform: "linux/arm64" },
  ],
  keyFile: "verity-apk-rsa.pub",
  keyFingerprint: "pending publication",
  caveat:
    "The Verity APK repository is experimental. Do not rely on it for production until the publish workflow has produced signed APKINDEX files and the repository verification task confirms availability.",
};
