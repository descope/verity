# Verity Catalog Analysis — Vulnerability Gaps & Competitive Positioning

## Executive Summary

| Metric | Value |
|--------|-------|
| **Total images** | 156 across 14 categories |
| **Integer (Wolfi-based, 0 CVE)** | 91 |
| **Copa-patched (upstream)** | 65 |
| **Copa images with residual CVEs** | 5 (valkey ×2, elasticsearch, cockroachdb, postgres-operator) |
| **Copa images at risk (0 CVE today, fragile base)** | ~15 (wordpress, jenkins, keycloak, etc.) |
| **Missing high-demand images** | 40+ vs competitors |
| **Biggest competitive blind spot** | AI/ML — zero images vs Chainguard/Bitnami offering 15+ each |

### Competitor Scale

| Competitor | Image Count | Approach | Zero-CVE? | FIPS |
|-----------|------------|---------|-----------|------|
| **Chainguard** | 2,329 | Wolfi from-source builds | Yes (SLA) | 400+ variants |
| **RapidFort** | 25,000+ | Profile & minimize upstream | 95% reduction | Yes (140-3) |
| **Bitnami** | 344 | Wolfi-based (migrated from Debian) | Near-zero | No |
| **Iron Bank** | 1,000+ | DISA STIG hardening | No (ORA scored) | Yes |
| **Docker Official** | 179 | Upstream, not hardened | No | No |
| **Verity** | 156 | Wolfi rebuilds + Copa patching | Yes (Integer), mostly (Copa) | Yes |

---

## Part 1: Copa-Patched Images with Residual Vulnerabilities

### Active Vulnerabilities (Non-Zero on verity.supply)

| Image | Residual Vulns | Root Cause | Recommendation |
|-------|---------------|------------|----------------|
| **valkey/valkey** | High | Upstream Debian/Ubuntu base; deep OS CVEs Copa can't reach | **Retire** — Wolfi `valkey` already exists with 0 CVE |
| **valkey/valkey-bundle** | High | Same as valkey | **Retire** — redirect to Wolfi variant |
| **elasticsearch** | High (~86K) | Complex Java/JDK image; many transitive native deps | **Wolfi rebuild** — challenging but highest impact |
| **cockroachdb** | Medium (~13K) | Go binary + Ubuntu; Copa patches OS but not Go deps | **Wolfi rebuild** — Go binary patching pipeline now live |
| **postgres-operator** (Zalando) | Medium (~13K) | Go binary on distroless-ish base | **Wolfi rebuild** — Go binary approach |

### Validated Migration Pattern (Copa → Wolfi)

These 12+ images were already successfully converted because Copa left unfixable CVEs:

| Image | Copa Problem | Now |
|-------|-------------|-----|
| nginx | Unfixable CVEs in Debian base | ✅ Wolfi |
| alertmanager | -distroless variant still had CVEs | ✅ Wolfi |
| cilium | Copa-patched Ubuntu base | ✅ Wolfi |
| coredns | Can't patch scratch-based image | ✅ Wolfi |
| cosign | Can't patch distroless image | ✅ Wolfi |
| envoy | Official image ships with unfixable CVEs | ✅ Wolfi |
| erlang | Debian with persistent unfixable CVEs | ✅ Wolfi |
| etcd | Can't patch Alpine/scratch image | ✅ Wolfi |
| prometheus | Can't patch scratch-based image | ✅ Wolfi |
| traefik | Can't fully patch Alpine | ✅ Wolfi |
| calico-cni | Copa-patched Alpine had residual CVEs | ✅ Wolfi |
| haproxy-ingress | Copa couldn't fully patch | ✅ Wolfi |

### Copa Images at Risk (0 CVE Today, Fragile Base)

Currently clean but on Debian/Ubuntu bases that regularly accumulate new CVEs:

| Image | Base | Risk | Priority |
|-------|------|------|----------|
| **wordpress** | Debian, huge attack surface (PHP + Apache) | High | High — top-30 Docker Hub |
| **jenkins** | Debian + JDK, complex | High | High — enterprise staple |
| **keycloak** | Java/Quarkus runtime | Medium | Medium |
| **sonarqube** | Debian + JDK | Medium | Medium |
| **tomcat** | Debian + JDK | Medium | Medium |
| **google-cloud-sdk** | Debian, very large | Medium | Medium |
| **mongodb** | Ubuntu | Medium | Medium |
| **airflow** | Debian + Python, massive dep tree | High | Medium |
| **renovate** | Node.js + Debian | Low | Low (dev tool) |
| **powershell** | Ubuntu/Debian | Low | Low |

### All 65 Copa-Patched Images by Category

<details>
<summary>Click to expand full Copa image list</summary>

**Web Servers & Proxies (4)**
- kubernetes/ingress-nginx/controller, kube-webhook-certgen, defaultbackend
- library/tomcat

**Databases & Caching (7)**
- valkey/valkey, valkey/valkey-bundle
- opensearchproject/opensearch, opensearch-dashboards
- cockroachdb/cockroach, clickhouse/clickhouse-server
- rqlite/rqlite, zalando/postgres-operator

**Messaging (2)**
- rabbitmqoperator/messaging-topology-operator, confluentinc/cp-kafka

**Kubernetes & Orchestration (12)**
- kubernetes/autoscaler/cluster-autoscaler, kubernetes-sigs/external-dns
- emberstack/kubernetes-reflector, kubernetes-sigs/secrets-store-csi-driver
- googlecloudplatform/secrets-store-csi-driver-provider-gcp
- kiwigrid/k8s-sidecar, kubernetes-sigs/node-feature-discovery
- rancher/k3s, crossplane/crossplane
- aws/eks-distro/* (kube-apiserver, kube-scheduler, kube-proxy, csi/node-driver-registrar)

**Service Mesh (1)** — cilium/hubble-ui

**Monitoring (8)**
- prometheus/node-exporter, mysqld-exporter
- prometheus-operator/prometheus-config-reloader
- prometheuscommunity/elasticsearch-exporter
- grafana/grafana-operator, grafana/promtail
- datadog/agent, datadog/cluster-agent
- victoriametrics/victoria-logs

**Logging (1)** — fluent/fluent-operator

**CI/CD (4)** — argoproj/argocd, jenkins/jenkins, tektoncd/cli, renovatebot/renovate

**Security (5)** — hashicorp/consul, keycloak/keycloak, spiffe/spiffe-helper, gravitational/teleport, openbao/openbao

**Policy (2)** — kyverno/policy-reporter-ui, openpolicyagent/gatekeeper

**Cert Management (5)** — cert-manager-controller, cainjector, acmesolver, cmctl, openshift-routes

**Data/ML (3)** — apache/airflow, kubeflow/spark-operator, mlflow/mlflow

**Base & Utilities (11)** — distroless/static, busybox, bash, google/cloud-sdk, powershell, hugo, wordpress, sonarqube, sonar-scanner-cli, ntpd-rs, library/docker (docker-cli)

</details>

---

## Part 2: Popular Images Missing From the Catalog

Cross-referenced against Chainguard (2,329), Bitnami (344), RapidFort (25K+), Docker Hub top-50, and Iron Bank.

### Tier 1 — Table Stakes Gaps (Top Docker Hub images you don't have)

| Image | Docker Hub Pulls | Who Has It | Category | Feasibility |
|-------|-----------------|-----------|----------|-------------|
| **mysql** | 1B+ | Chainguard, RapidFort, Bitnami, Iron Bank | Database | Medium — MariaDB done, similar approach |
| **redis** | 1B+ | Chainguard, RapidFort, Bitnami, Iron Bank | Database | Easy — Valkey fork already Wolfi-based |
| **cassandra** | 100M+ | Chainguard, RapidFort, Bitnami | Database | Hard — complex Java + native |
| **neo4j** | 100M+ | Chainguard, RapidFort, Bitnami | Database | Hard — JVM + plugins |
| **couchdb** | 50M+ | RapidFort, Bitnami | Database | Medium — Erlang (you have Erlang Wolfi) |
| **mongo-express** | 100M+ | Docker Official | Database UI | Easy — Node.js app |
| **kong** | 50M+ | Bitnami, Docker Official | Proxy | Medium |
| **gitlab-runner** | 500M+ | Chainguard, Bitnami, RapidFort | CI/CD | Medium — Go binary |
| **actions-runner** | Growing | Chainguard | CI/CD | Medium — Go/Node hybrid |
| **harbor** (8 components) | 100M+ | Bitnami | Registry | Medium — Go services |
| **nextcloud** | 1B+ | Docker Official, Iron Bank | CMS | Hard — PHP + many deps |
| **registry** (Docker) | 1B+ | Docker Official | Infrastructure | Easy — Go binary |
| **containerd** | - | Standard K8s dep | Runtime | Easy — Go binary |

### Tier 2 — AI/ML (Your #1 Competitive Blind Spot)

Verity has **3 Copa-patched ML images** (airflow, spark-operator, mlflow). Chainguard has an entire AI category. Bitnami has 15+ AI/ML images.

| Image | Who Has It | Why Critical |
|-------|-----------|-------------|
| **pytorch** | Chainguard, Bitnami | #1 ML framework, massive user base |
| **tensorflow** / tensorflow-serving | Chainguard, Bitnami | Enterprise ML staple |
| **jupyter-base-notebook** | Bitnami | Standard data science environment |
| **jupyterhub** | Bitnami | Multi-user Jupyter |
| **ollama** | Chainguard | Local LLM inference — exploding in popularity |
| **vllm** | Chainguard | Production LLM serving |
| **kserve** (6 components) | Bitnami | ML model serving on K8s |
| **kuberay** (operator + apiserver) | Bitnami | Ray on K8s |
| **milvus** | Bitnami | Vector database for LLM apps |
| **triton-inference-server** | Chainguard | NVIDIA's inference server |
| **deepspeed** | Bitnami | Distributed training |
| **cuda** (base) | Chainguard | GPU workload base image |

> **Impact**: Any enterprise doing ML (which is now nearly all of them) will find Verity's catalog insufficient and default to Chainguard.

### Tier 3 — Observability Gaps (Incomplete Stack)

You have Prometheus, Grafana, Loki, Mimir, Thanos, Telegraf, Vector. Missing:

| Image | Who Has It | Why You Need It |
|-------|-----------|-----------------|
| **opentelemetry-collector** | Bitnami, Chainguard | Industry-standard telemetry — Bitnami has ALL OTel components |
| **jaeger** | Bitnami, RapidFort | CNCF distributed tracing |
| **tempo** (Grafana) | Bitnami | Trace backend — completes your Grafana suite |
| **victoria-metrics** (server) | Bitnami (full VM stack) | You have victoria-logs but not the metrics server |
| **pyroscope** (Grafana) | Bitnami | Continuous profiling — Grafana suite |
| **grafana-alloy** | Bitnami, Iron Bank | OpenTelemetry distribution for Grafana |
| **grafana-k6** | Bitnami | Load testing |
| **blackbox-exporter** | Bitnami, RapidFort | Prometheus probing |
| **kibana** | Bitnami, RapidFort, Iron Bank | Elasticsearch frontend |
| **elastic-agent** | Bitnami | Elastic observability |

### Tier 4 — Security & Identity Gaps

| Image | Who Has It | Gap Impact |
|-------|-----------|-----------|
| **trivy** | Chainguard (free for 1 year!), RapidFort | Meta — a vuln scanner should itself be zero-CVE |
| **falco** | Iron Bank, Bitnami | Runtime security — CNCF graduated |
| **dex** | Chainguard | OIDC IdP — connects ArgoCD, K8s OIDC, etc. |
| **oauth2-proxy** | Bitnami | Universal auth proxy |
| **cert-manager** (Wolfi variant) | Bitnami, Chainguard | You Copa-patch it; competitors offer zero-CVE builds |
| **external-secrets** | Bitnami | K8s secrets management — growing fast |
| **sealed-secrets** | Bitnami | GitOps-friendly secrets |
| **checkov** | Bitnami | IaC security scanning |
| **pinniped** | Bitnami | K8s auth |

### Tier 5 — Kubernetes Ecosystem Gaps

| Image | Who Has It | Why |
|-------|-----------|-----|
| **metrics-server** | Bitnami | Core K8s component |
| **velero** (+ plugins) | Bitnami | Backup/restore — enterprise essential |
| **flux** (6 controllers) | Bitnami, Chainguard | GitOps — competitor to your ArgoCD |
| **metallb** | Bitnami | Bare-metal LB — growing |
| **multus-cni** | Bitnami | Multi-network K8s |
| **longhorn** | Chainguard | Rancher storage |
| **local-path-provisioner** | Bitnami | Development storage |
| **kaniko** | Bitnami | In-cluster builds |

### Tier 6 — Application Platforms & Dev Tools

| Image | Who Has It | Demand Signal |
|-------|-----------|--------------|
| **mattermost** | Iron Bank | Enterprise chat, DoD-approved |
| **gitea/forgejo** | Bitnami | Self-hosted Git — growing alternative to GitLab |
| **metabase** | RapidFort, Iron Bank | BI dashboards |
| **temporal** (server + CLI + UI) | Bitnami | Durable execution engine — rising fast |
| **supabase** | Growing | Open-source Firebase |
| **superset** | Bitnami | BI/analytics |
| **appsmith** | Bitnami | Low-code platform |
| **drupal** | Bitnami, Docker Official | CMS |
| **ghost** | Bitnami, Docker Official | Publishing platform |
| **phpmyadmin** | Bitnami, Docker Official | MySQL admin |
| **mastodon** | Bitnami | Federated social |

### Tier 7 — Storage & Data Infrastructure

| Image | Who Has It | Why |
|-------|-----------|-----|
| **scylladb** | Bitnami | Cassandra-compatible, growing |
| **timescaledb** | — | Time-series PostgreSQL extension |
| **meilisearch** | — | Full-text search, developer-friendly |
| **typesense** | — | Search engine |
| **seaweedfs** | Bitnami | Distributed storage |
| **solr** | Bitnami, RapidFort, Iron Bank | Enterprise search |

### New Category: MCP (Machine Communication Protocol)

Bitnami is already shipping MCP images — a brand-new category:
- **mcp-grafana**, **mcp-redis**, **mcp-mongodb**
- Chainguard has an "MCP" category label

This is an emerging trend worth watching.

---

## Part 3: Strategic Recommendations

### 🔴 Immediate (This Sprint)

| # | Action | Impact | Effort |
|---|--------|--------|--------|
| 1 | **Retire Copa valkey/valkey & valkey-bundle** — Wolfi variant already exists at 0 CVE | Eliminates highest-vuln images from catalog | Low |
| 2 | **Convert elasticsearch → Wolfi** | Removes ~86K residual vulns | High |
| 3 | **Convert cockroachdb → Wolfi** (Go binary patching) | Removes ~13K residual vulns | Medium |
| 4 | **Convert postgres-operator → Wolfi** (Go binary patching) | Removes ~13K residual vulns | Medium |

### 🟡 Short-Term (Next Quarter)

| # | Action | Impact | Effort |
|---|--------|--------|--------|
| 5 | **Launch AI/ML category** — pytorch, jupyter, ollama minimum | Closes biggest competitive gap | High |
| 6 | **Add mysql** | Table stakes — 1B+ Docker Hub pulls | Medium |
| 7 | **Add redis** (official, not just Valkey) | Table stakes — 1B+ pulls | Easy |
| 8 | **Add opentelemetry-collector** | Cloud-native observability standard | Medium |
| 9 | **Add trivy** | Security product credibility | Easy |
| 10 | **Convert wordpress/jenkins/keycloak → Wolfi** | Reduce Copa fragility risk | High |

### 🟢 Medium-Term (Next 2 Quarters)

| # | Action | Impact | Effort |
|---|--------|--------|--------|
| 11 | **Complete observability** — jaeger, tempo, alloy, pyroscope, blackbox-exporter | Full Grafana/OTel stack | Medium |
| 12 | **Add CI/CD images** — gitlab-runner, actions-runner, kaniko | Common enterprise need | Medium |
| 13 | **Add security images** — trivy, falco, dex, oauth2-proxy, external-secrets | Complete security story | Medium |
| 14 | **Add harbor / registry** | Self-hosted registry demand | Medium |
| 15 | **Add kubernetes ecosystem** — metrics-server, velero, flux controllers | Enterprise K8s gaps | Medium |
| 16 | **Add application platforms** — temporal, metabase, gitea | Developer demand | Varies |

### Strategic Notes

1. **AI/ML is the #1 priority gap.** Chainguard and Bitnami have invested heavily here. Every enterprise running ML workloads will look elsewhere if Verity has nothing.

2. **Copa → Wolfi migration** has a proven track record (12+ successful conversions). The Go binary patching pipeline makes more images feasible now.

3. **Bitnami is the closest competitor in approach** — they migrated from Debian to Wolfi and now offer 344 images with near-zero CVEs. Their full OTel stack, VictoriaMetrics suite, and MCP images show where the market is heading.

4. **Chainguard at 2,329 images is 15x your catalog.** You can't match breadth, but you can win on depth (FIPS, SLSA L3, cosign, Helm charts) in the categories you do cover.

5. **RapidFort at 25K+ images wins on breadth alone** but their approach (minimize upstream) is fundamentally different from Wolfi rebuilds. Their DoD/Iron Bank integration is notable.
