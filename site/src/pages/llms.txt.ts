import type { APIRoute } from "astro";
import { copaCount, integerCount, totalCategories, totalImages } from "../data/full-catalog.ts";

export const GET: APIRoute = ({ site }) => {
  const base = import.meta.env.BASE_URL;
  const origin = site?.origin ?? "https://verity.supply";
  const prefix = `${origin}${base}`;

  const content = `# Verity

> Self-maintaining registry of security-patched container images. Verity continuously scans container images for CVEs, patches them in-place using Copa (no Dockerfile rebuild required), signs with cosign/Sigstore keyless OIDC, attests with SLSA L3 build provenance and CycloneDX SBOMs, and publishes signed drop-in replacements to \`verity.supply\`.

Verity covers ${totalImages} container images across ${totalCategories} categories: languages & build tools, web servers & proxies, databases & caching, messaging & streaming, Kubernetes & orchestration, service mesh & networking, monitoring & observability, logging, CI/CD & GitOps, security & identity, policy & compliance, cert management, data & ML, and base & utilities.

Images are available in two forms: **Copa-patched** (${copaCount} images — in-place OS-level package patching of upstream images) and **Wolfi-based** (${integerCount} images — from-scratch hardened rebuilds using Wolfi packages with minimal attack surface). All images support linux/amd64 and linux/arm64. FIPS variants are available for Go-backed images (golang, caddy, docker-cli, etcd, helm, terraform, cosign, crane, pgweb, vault) and OpenSSL-backed images (node, nginx, php, ruby, erlang, haproxy, httpd, python, postgres, valkey, dotnet).

The patching pipeline runs daily at 02:00 UTC via GitHub Actions. Every published image carries five supply-chain attestations: cosign keyless signature, SLSA Level 3 build provenance, CycloneDX SBOM, Trivy vulnerability report, and a Rekor transparency log entry.

Replace your image reference — that's it:
\`\`\`
docker pull verity.supply/prometheus/prometheus:v3.9.1-patched
\`\`\`

## Documentation

- [Complete LLM Reference](${prefix}llms-full.txt): Full documentation in a single file — project overview, architecture, complete image catalog with all ${totalImages} images, compliance framework mappings, CLI reference, configuration format, and contributing guide
- [Overview & Quick Start](${prefix}index.md): What Verity is, how to use patched images, catalog summary by category
- [Supply Chain Compliance](${prefix}compliance.md): SLSA L3, Sigstore/cosign, CycloneDX SBOM, Rekor transparency log, plus framework mappings for FedRAMP/NIST 800-53, SOC 2, ISO 27001, OWASP ASVS, NIST CSF 2.0, and CISA Secure by Design
- [Helm Charts](${prefix}charts/index.md): Pre-patched wrapper Helm charts that override upstream image references with Verity's patched equivalents
- [Experimental APK Repository](${prefix}apk/index.md): Pending APK repository entry point with install instructions, key rotation notes, supported arches, and experimental caveats

## Source

- [GitHub Repository](https://github.com/verity-org/verity): Source code, CI/CD pipeline, issue tracker, discussions
- [Browse Image Catalog](${prefix}): Interactive catalog with vulnerability data, severity breakdowns, and supply chain badges
`;

  return new Response(`${content.trim()}\n`, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
      "Cache-Control": "public, max-age=3600",
    },
  });
};
