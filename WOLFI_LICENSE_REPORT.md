# Wolfi APK Package Availability & License Verification Report
## Integer Image Build Analysis - March 16, 2026

---

## SUMMARY TABLE

| Project | Wolfi Package | Status | Upstream License | Redistribution Risk |
|---------|---------------|--------|------------------|-------------------|
| configmap-reload | ✅ configmap-reload | Available | Apache-2.0 | ✅ OK |
| crossplane | ✅ crossplane-2.2 | Available | Apache-2.0 | ✅ OK |
| promtail | ❌ NOT AVAILABLE | Missing | **AGPL-3.0** | 🚨 **BLOCKED** |
| spire-server | ✅ spire-server | Available | Apache-2.0 | ✅ OK |
| spire-agent | ✅ spire-agent (subpkg) | Available | Apache-2.0 | ✅ OK |
| kyverno | ✅ kyverno-1.17 | Available | Apache-2.0 | ✅ OK |
| haproxy-ingress | ✅ haproxy-ingress | Available | Apache-2.0 | ✅ OK |
| istio | ✅ istio-1.29 | Available | Apache-2.0 | ✅ OK |
| cilium | ✅ cilium-1.19 | Available | Apache-2.0 | ✅ OK |
| calico | ✅ calico-3.31 | Available | Apache-2.0 | ✅ OK |
| newrelic-infrastructure-bundle | ✅ newrelic-infrastructure-bundle | Available | Apache-2.0 | ✅ OK |
| logstash-oss | ✅ logstash-9.3 (OSS-only) | Available | Apache-2.0 | ✅ OK |
| fluentd | ✅ ruby*-fluentd | Available | Apache-2.0 | ✅ OK |
| strimzi-kafka | ✅ strimzi-kafka-operator | Available | Apache-2.0 | ✅ OK |
| eck-operator | ❌ NOT AVAILABLE | Missing | Elastic License 2.0 | 🚨 **PROPRIETARY** |
| node-exporter | ❌ NOT AVAILABLE | Missing | Apache-2.0 | ⚠️ **NO PKG** |

---

## CRITICAL FINDINGS

### ✅ CORRECTED: spire-agent IS AVAILABLE
The `spire-server` package in Wolfi includes **subpackages** for both:
- ✅ **spire-agent** (binary: `/usr/bin/spire-agent`)
- ✅ **spire-oidc-discovery-provider** (binary: `/usr/bin/oidc-discovery-provider`)

All components are Apache-2.0 licensed and ready for Integer.

### ✅ VERIFIED: logstash-9.3 is OSS-only in Wolfi
The Wolfi build recipe explicitly:
1. Sets `OSS: "true"` environment variable
2. **REMOVES x-pack folder** during build: `rm -rf "$tmpdir"/logstash-${{package.version}}-SNAPSHOT/x-pack`
3. Uses `logstash-oss.yml` configuration
4. This makes the output fully Apache-2.0 compliant for redistribution

---

## DETAILED FINDINGS

### ✅ AVAILABLE & SAFE FOR REDISTRIBUTION (Apache-2.0)

#### 1. configmap-reload
- **Wolfi Package:** `configmap-reload` (v0.15.0)
- **Upstream Repo:** jimmidyson/configmap-reload
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/configmap-reload.yaml
- **Status:** ✅ Ready to use

#### 2. crossplane
- **Wolfi Package:** `crossplane-2.2` (v2.2.0)
- **Upstream Repo:** crossplane/crossplane
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/crossplane-2.2.yaml
- **Status:** ✅ Ready to use

#### 3. spire-server + spire-agent (subpackage)
- **Wolfi Package:** `spire-server` (v1.14.1)
- **Wolfi Subpackages:** 
  - ✅ `spire-agent` — installed to `/usr/bin/spire-agent`
  - ✅ `spire-oidc-discovery-provider` — installed to `/usr/bin/oidc-discovery-provider`
- **Upstream Repo:** spiffe/spire
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/spire-server.yaml
- **Status:** ✅ Ready to use (all components available)

#### 4. kyverno (with all subcomponents)
- **Wolfi Package:** `kyverno-1.17` (v1.17.1)
- **Upstream Repo:** kyverno/kyverno
- **License:** Apache-2.0
- **Subcomponents Available:**
  - ✅ `kyverno-init-container-1.17`
  - ✅ `kyverno-background-controller-1.17`
  - ✅ `kyverno-reports-controller-1.17`
  - ✅ `kyverno-cleanup-controller-1.17`
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/kyverno-1.17.yaml
- **Status:** ✅ All subcomponents available

#### 5. haproxy-ingress
- **Wolfi Package:** `haproxy-ingress` (v0.15.1)
- **Upstream Repo:** jcmoraisjr/haproxy-ingress
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/haproxy-ingress.yaml
- **Status:** ✅ Ready to use

#### 6. istio (with all subcomponents)
- **Wolfi Package:** `istio-1.29` (v1.29.0)
- **Upstream Repo:** istio/istio
- **License:** Apache-2.0
- **Subcomponents Available:**
  - ✅ `istio-cni-1.29` (binary: `istio-cni`)
  - ✅ `istio-install-cni-1.29` (binary: `install-cni.sh`)
  - ✅ `istio-pilot-discovery-1.29` (binary: `pilot-discovery`)
  - ✅ `istio-pilot-agent-1.29` (binary: `pilot-agent`)
  - ✅ `istioctl-1.29` (binary: `istioctl`)
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/istio-1.29.yaml
- **Status:** ✅ All subcomponents available

#### 7. cilium
- **Wolfi Package:** `cilium-1.19` (v1.19.1)
- **Upstream Repo:** cilium/cilium
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/cilium-1.19.yaml
- **Status:** ✅ Ready to use

#### 8. calico (with all subcomponents)
- **Wolfi Package:** `calico-3.31` (v3.31.4)
- **Upstream Repo:** projectcalico/calico
- **License:** Apache-2.0
- **Subcomponents Available:**
  - ✅ `calico-node-3.31` (binary: `calico-node`)
  - ✅ `calico-felix-3.31` (binary: `felix`)
  - ✅ `calico-cni-3.31` (binaries: `calico`, `calico-ipam`)
  - ✅ `calico-apiserver-3.31` (binary: `calico-apiserver`)
  - ✅ `calico-kube-controllers-3.31` (binary: `kube-controllers`)
  - ✅ `calico-key-cert-provisioner-3.31` (binary: `key-cert-provisioner`)
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/calico-3.31.yaml
- **Status:** ✅ All subcomponents available

#### 9. newrelic-infrastructure-bundle
- **Wolfi Package:** `newrelic-infrastructure-bundle` (v3.3.16)
- **Upstream Repo:** newrelic/infrastructure-bundle
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/newrelic-infrastructure-bundle.yaml
- **Status:** ✅ Ready to use

#### 10. logstash (OSS-only build)
- **Wolfi Package:** `logstash-9.3` (v9.3.1)
- **Upstream Repo:** elastic/logstash
- **License:** Apache-2.0 (Wolfi builds OSS variant only)
- **Wolfi Build Configuration:**
  - Explicitly sets `OSS: "true"`
  - **Removes x-pack folder** during build (proprietary components excluded)
  - Uses `logstash-oss.yml` configuration
  - X-pack license is NOT included in the build output
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/logstash-9.3.yaml
- **Status:** ✅ Safe for redistribution (fully Apache-2.0)

#### 11. fluentd
- **Wolfi Packages:** 
  - `ruby3.2-fluentd` (v1.19.x)
  - `ruby3.3-fluentd` (v1.17.x)
  - `ruby3.4-fluentd` (v1.17.x)
  - `ruby4.0-fluentd` (v1.17.x)
  - `ruby*-fluentd-kubernetes-daemonset` (multiple versions)
- **Upstream Repo:** fluent/fluentd
- **License:** Apache-2.0
- **Wolfi Builds:** https://github.com/wolfi-dev/os/blob/main/ruby3.*.yaml (fluentd packages)
- **Status:** ✅ Ready to use

#### 12. strimzi-kafka-operator
- **Wolfi Package:** `strimzi-kafka-operator` (v0.50.1)
- **Upstream Repo:** strimzi/strimzi-kafka-operator
- **License:** Apache-2.0
- **Wolfi Build:** https://github.com/wolfi-dev/os/blob/main/strimzi-kafka-operator.yaml
- **Status:** ✅ Ready to use

---

## ❌ CRITICAL ISSUES - CANNOT REDISTRIBUTE

### 1. promtail (from Grafana Loki)
- **Wolfi Status:** ❌ **NOT PACKAGED IN WOLFI**
- **Upstream Repo:** grafana/loki
- **License:** **AGPL-3.0** (GNU Affero General Public License v3)
- **Upstream License URL:** https://github.com/grafana/loki/blob/main/LICENSE
- **🚨 COPYLEFT LICENSE - CANNOT BE INCLUDED IN INTEGER**
- **Why AGPL-3.0 Blocks Redistribution:**
  - AGPL-3.0 is a copyleft license requiring source code disclosure
  - If promtail is used as a network service (which it is), the entire aggregate must be open-sourced
  - This creates compliance obligations incompatible with closed/proprietary Integer distributions
- **Recommendation:**
  - ❌ Do NOT include promtail in Integer images
  - Consider alternatives:
    - Build custom log forwarding using open-source tools
    - Use other Apache-2.0 licensed log collectors
    - If Loki logs are essential, ensure AGPL compliance strategy is in place

### 2. eck-operator (Elastic Cloud on Kubernetes)
- **Wolfi Status:** ❌ **NOT PACKAGED IN WOLFI**
- **Upstream Repo:** elastic/cloud-on-k8s
- **License:** **Elastic License 2.0** (proprietary, non-open-source)
- **Upstream License URL:** https://github.com/elastic/cloud-on-k8s/blob/main/LICENSE.txt
- **🚨 PROPRIETARY LICENSE - CANNOT BE INCLUDED IN INTEGER**
- **Why Elastic License 2.0 Blocks Redistribution:**
  - Elastic License 2.0 is a proprietary license NOT compatible with open-source
  - Prohibits production use without a paid Elastic license
  - Cannot be bundled into Integer distribution
- **Recommendation:**
  - ❌ Do NOT include eck-operator in Integer images
  - Consider alternatives:
    - Use open-source Kubernetes operators for Elasticsearch management
    - Implement custom controller with Apache-2.0 license
    - If Elasticsearch is needed, use standalone (non-operator) approach

### 3. node-exporter (Prometheus)
- **Wolfi Status:** ❌ **NO STAND-ALONE PACKAGE IN WOLFI**
- **Upstream Repo:** prometheus/node_exporter
- **Upstream License:** Apache-2.0 (permissive, safe for redistribution)
- **Issue:** Prometheus node_exporter is not independently packaged in Wolfi
  - Searched `/tmp/wolfi-os/*.yaml` for node-exporter packages
  - Only `kube-logging-operator-node-exporter` exists (different project, not prometheus node_exporter)
  - Prometheus packages exist (prometheus-3.9.yaml) but don't include node-exporter binary
- **Recommendation:**
  - File PR with wolfi-dev/os to add `prometheus-node-exporter` package
  - Alternatively: use prebuilt node-exporter binary from Prometheus releases
  - Or use monitoring stack that includes node metrics (e.g., Cilium includes node-exporter)

---

## LICENSING COMPLIANCE SUMMARY

### 🟢 SAFE FOR INTEGER REDISTRIBUTION (11 projects)
All Apache-2.0, fully permissive, no compliance issues:
1. ✅ configmap-reload
2. ✅ crossplane
3. ✅ spire-server + spire-agent (subpackage)
4. ✅ kyverno + subcomponents
5. ✅ haproxy-ingress
6. ✅ istio + all subcomponents
7. ✅ cilium
8. ✅ calico + all subcomponents
9. ✅ newrelic-infrastructure-bundle
10. ✅ logstash-oss (Wolfi build verified to be OSS-only)
11. ✅ fluentd
12. ✅ strimzi-kafka-operator

### 🔴 BLOCKED FROM INTEGER REDISTRIBUTION (2 projects)
Copyleft or proprietary licenses prevent redistribution:
1. ❌ **promtail** — AGPL-3.0 (copyleft, network service clause)
2. ❌ **eck-operator** — Elastic License 2.0 (proprietary)

### ⚠️ NOT AVAILABLE IN WOLFI (1 project)
Open-source license (Apache-2.0) but not packaged:
1. ⚠️ **node-exporter** — Needs Wolfi packaging or alternative solution

---

## IMPLEMENTATION CHECKLIST FOR INTEGER

### Phase 1: Deploy Immediately (Safe & Available)
- [ ] configmap-reload
- [ ] crossplane
- [ ] spire-server (includes spire-agent subpackage)
- [ ] kyverno (with all subcomponents)
- [ ] haproxy-ingress
- [ ] istio (with all subcomponents)
- [ ] cilium
- [ ] calico (with all subcomponents)
- [ ] newrelic-infrastructure-bundle
- [ ] logstash-oss
- [ ] fluentd
- [ ] strimzi-kafka-operator

### Phase 2: Action Required
- [ ] **node-exporter:** Evaluate alternatives OR submit Wolfi PR for prometheus-node-exporter
- [ ] **promtail:** Exclude from Integer OR implement AGPL-3.0 compliance strategy
- [ ] **eck-operator:** Exclude from Integer OR find open-source Elasticsearch operator alternative

---

## VERIFICATION COMMANDS FOR DEPLOYMENTS

```bash
# Verify available Wolfi packages
apk search configmap-reload crossplane kyverno haproxy-ingress \
  istio cilium calico newrelic-infrastructure-bundle logstash \
  fluentd strimzi-kafka-operator spire-server

# Check spire-agent is available as subpackage
apk search spire-agent

# Verify all components are Apache-2.0 by checking package metadata
apk info -L <package-name> | grep -i license
```

