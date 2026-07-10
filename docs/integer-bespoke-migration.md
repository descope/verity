# Integer Bespoke Recipe Migration

## Decision

Verity now vendors source-maintained melange recipes in-repo for Integer packages that need rebuilds. Recipe sourcing is fully local. The external APK build/runtime dependency repository remains in scope only as a dependency source; bootstrapping a full distro is not part of this migration.

## Layout

- `packages/bespoke/locked/<recipe>.yaml`: vendored baseline recipe YAML, maintained by Verity.
- `packages/bespoke/locked/<recipe>/...`: sidecar files used by that recipe, for example patches or static assets.
- `packages/pipelines/...`: minimal vendored melange pipeline YAMLs required by vendored recipes.
- `packages/bespoke/*.yaml`: Verity-authored recipes that are independent bespoke recipes.

Every package already tracked in `packages/upstream.lock.json` has a corresponding
local recipe. `TestLockedRecipeInventoryComplete` prevents future lock entries from
being added without their recipe. The pilot-specific files are:

- `packages/bespoke/locked/caddy.yaml` bumped to `2.11.4-r3` plus `packages/bespoke/locked/caddy/{Caddyfile,index.html}`.
- `packages/bespoke/locked/cilium-1.19.yaml` plus `packages/bespoke/locked/cilium-1.19/loopback-location.patch`.
- The transitive local pipeline closure used by all locked recipes.

## Mechanism

`packages/upstream.lock.json` now treats `packages.<name>.file` as a path relative to `packages/bespoke/locked/`, verifies recipes, sidecars, and shared pipelines by SHA-256, and stores provenance under `provenance.recipe_baseline_commit` instead of using it for live fetches.

Local prep (`scripts/integer-melange-prep.sh`), CI build (`.github/scripts/melange-build.sh`), and workflow prep (`integer-build-image.yaml`) all stage recipes from `packages/bespoke/locked/`. They reject missing, unlisted, modified, symlinked, or otherwise non-regular recipes, sidecars, and shared pipelines before staging them. Grep validation must stay clean for legacy live recipe URLs and remotes, the old pipeline-fetch helper, and shell network fetches in these paths.

Images opt into local rebuild by declaring `types.<type>.melange.upstream`. `images/cilium.yaml` now uses `upstream: "cilium-{{version}}"` and declares an explicit `1.19` version key, so `cilium:1.19-default` builds `packages/bespoke/locked/cilium-1.19.yaml` before image assembly.

## CVE Tracking

Copying recipes is only a starting point. Each vendored lock entry must record:

- source recipe provenance (`source`).
- maintained upstream version (`version` / `upstream`).
- SHA-256 manifests for the recipe, every sidecar, and every shared pipeline.
- `cve_tracking` note explaining package version bumps or dependency bumps tied to the Trivy gate.

Pilot CVE-driven bumps: `caddy` was bumped to `2.11.4-r3` after pilot gates reported `CVE-2026-42505`, `CVE-2026-39822`, and `CVE-2026-41178`; epoch `3` includes real `go.opentelemetry.io/otel` remediation to `v1.44.0`; `cilium-1.19` was vendored from public baseline `1.19.1-r1` and bumped to upstream `v1.19.5` (`expected-commit: 20eaccfef029cb046d70219717fb6dbbdf27a59f`) to target the failing `cilium:1.19-default` gate. The hard-case pilot also carries epoch `5` with real bundled Go dependency remediation: `go.opentelemetry.io/otel`, `otel/trace`, and `otel/metric` to `v1.44.0`, `otel/exporters/otlp/otlptrace/otlptracehttp` to `v1.43.0`, and `go.mongodb.org/mongo-driver` to `v1.17.7`, followed by `go mod vendor` because Cilium builds with its vendor tree. Epoch `5` also guarantees the locked bespoke package outranks any same-version remote package. The recipe also normalizes `/var/lib` permissions and pre-creates writable `/var/lib/db/sbom` in main and clustermesh subpackage outputs so melange can preserve xattrs and write SBOM metadata on aarch64.

## Pilot Evidence

Local validation completed on the rebased branch before pilot dispatch:

- `go test ./...` passed.
- `python3 .github/scripts/validate-integer-build-image-workflow.py` passed.
- `bash -n scripts/integer-melange-prep.sh .github/scripts/melange-build.sh .github/scripts/melange-check.sh` passed.
- Grep validation stayed clean for legacy live recipe-fetch markers.

Earlier CI proof runs established the package-level remediations before the
mechanism-hardening changes:

- `caddy:2-fips`: https://github.com/verity-org/verity/actions/runs/29141068098
  - `gate-amd64.tar` reported `Vulnerabilities 0`.
  - `gate-arm64.tar` reported `Vulnerabilities 0`.
- `cilium:1.19-default`: https://github.com/verity-org/verity/actions/runs/29141068643
  - `gate-amd64.tar` reported `Vulnerabilities 0`.
  - `gate-arm64.tar` reported `Vulnerabilities 0`.

Hard-case result: `cilium-1.19` did not clear on upstream `v1.19.5` alone. The final clean run required real bundled dependency remediation for OpenTelemetry-Go and MongoDB Go driver modules plus vendor regeneration, proving that Verity must maintain Go module patches, not only copy recipes or bump package versions.

## Feasibility Reproduction

Reproduced on 2026-07-10 from current GitHub Actions data since 2026-07-09 and live APKINDEX origin fields. Current data differs from the 2026-07-09 baseline: raw failing package names = 171, origin-corrected unique names = 158, recipes at current public HEAD = 0, old-only public recipes = 82, no public recipe = 76.

Old-only sample: apisix-ingress-controller, argo-cd-3.3, argo-rollouts, argo-workflows-4.0, argocd-image-updater, authservice, aws-eks-pod-identity-agent, aws-load-balancer-controller, aws-node-termination-handler, aws-otel-collector, aws-s3-controller, azure-workload-identity-webhook, bank-vaults, boring-registry, buildah, caddy, cadvisor, calico-3.31, cassandra-5.0, cert-manager-csi-driver

Current no-public-recipe list:

- `argo-cd-3.0`
- `argo-cd-3.1`
- `argo-cd-3.2`
- `argo-cd-3.4`
- `argo-workflows`
- `calico-3.30`
- `calico-3.32`
- `cert-manager-1.20`
- `checkov`
- `cilium-1`
- `cilium-1.17`
- `cilium-1.18`
- `crossplane`
- `crossplane-2`
- `crossplane-2.1`
- `etcd`
- `external-dns`
- `external-secrets-operator`
- `gradle-8`
- `grafana-11.6`
- `grafana-12.0`
- `grafana-12.1`
- `grafana-12.2`
- `grafana-12.4`
- `grafana-13.0`
- `grafana-mimir-3.1`
- `haproxy`
- `haproxy-ingress-0`
- `haproxy-ingress-0.15`
- `haproxy-ingress-0.16`
- `helm-3`
- `influxd-2.7`
- `jaeger`
- `kafka-4.0`
- `kafka-4.1`
- `karpenter-1.11`
- `keycloak-26.3`
- `keycloak-26.6`
- `kubernetes-1.26`
- `kubernetes-1.27`
- `kubernetes-1.28`
- `kubernetes-1.29`
- `kubernetes-1.30`
- `kubernetes-1.31`
- `kubernetes-1.32`
- `kubernetes-1.33`
- `kubernetes-csi-node-driver-registrar`
- `kyverno-1.18`
- `logstash-9-with-output-opensearch`
- `loki-3.7`
- `mariadb-10.11`
- `mariadb-10.6`
- `mariadb-11.4`
- `opensearch-2`
- `opentofu-1.10`
- `opentofu-1.12`
- `opentofu-1.9`
- `pgbouncer`
- `postgresql-12`
- `postgresql-13`
- `postgresql-14`
- `postgresql-15`
- `prometheus-3.10`
- `prometheus-3.11`
- `prometheus-3.2`
- `prometheus-3.4`
- `prometheus-3.5`
- `prometheus-3.6`
- `prometheus-3.7`
- `ruby3.2-fluentd-kubernetes-daemonset-1-kinesis`
- `ruby3.2-fluentd-kubernetes-daemonset-1.18`
- `tempo`
- `traefik-3.3`
- `traefik-3.4`
- `traefik-3.5`
- `traefik-3.7`

## Scaling Plan

Recommended batching:

1. Keep pilot batches small: 2-3 packages, one version-bump package minimum, one unchanged-copy smoke package, one sidecar-heavy package.
2. Prioritize old-only recipes before no-public recipes. Old-only work starts from a known melange shape but still requires version/dependency maintenance.
3. Split no-public recipes by ecosystem and owner: Kubernetes controllers, Grafana/observability, databases, Java, Ruby, and infrastructure CLIs.
4. Require each package PR to include recipe, sidecars, lock metadata, local melange build evidence when feasible, and CI Trivy gate URL.

Estimated cost:

- Old-only recipe with straightforward Go bump: 2-6 engineer-hours.
- Old-only recipe with sidecars/custom pipelines/large builds: 1-2 engineer-days.
- Hard Go package requiring bundled dependency remediation: 1-3 engineer-days per CVE batch.
- No-public recipe from scratch: 2-5 engineer-days.
- Hard packages with native extensions, Java/Ruby packaging, or large app distributions: 1-2 engineer-weeks.

Ongoing burden: monitor upstream releases and CVEs, bump `version` or vulnerable module deps, refresh expected commits and sidecars, keep minimal shared pipelines current, and re-run both-arch Trivy gates before publish. This is permanent distro-maintainer work, not a one-time copy.
