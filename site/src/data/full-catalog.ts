export const REGISTRY = "ghcr.io/verity-org";

export type ImageSource = "copa" | "integer" | "incompatible";

export interface FullCatalogImage {
  name: string;
  label?: string;
  source: ImageSource;
  upstream?: string;
  variants?: string[];
  note?: string;
}

/** Extract the path portion after the registry host from an upstream ref. */
export function upstreamPath(image: FullCatalogImage): string {
  if (image.upstream) {
    return image.upstream.split("/").slice(1).join("/");
  }
  return image.name;
}

export function stripTag(ref: string): string {
  const lastSlash = ref.lastIndexOf("/");
  const lastColon = ref.lastIndexOf(":");
  if (lastColon > lastSlash) {
    return ref.slice(0, lastColon);
  }
  return ref;
}

export interface FullCatalogCategory {
  id: string;
  label: string;
  images: FullCatalogImage[];
}

export const fullCatalog: FullCatalogCategory[] = [
  {
    id: "languages",
    label: "Languages & Build Tools",
    images: [
      { name: "golang", source: "integer", variants: ["default", "dev", "fips"] },
      { name: "python", source: "integer", variants: ["default", "dev"] },
      { name: "node", source: "integer", variants: ["default", "dev"] },
      { name: "rust", source: "integer", variants: ["default", "dev"] },
      { name: "ruby", source: "integer", variants: ["default", "dev"] },
      { name: "dotnet", source: "integer", variants: ["default", "dev"] },
      { name: "erlang", source: "integer" },
      { name: "openjdk", source: "integer", variants: ["default", "dev"] },
      { name: "php", source: "integer", variants: ["default", "dev"] },
      { name: "deno", source: "integer" },
      { name: "gcc", source: "integer" },
      {
        name: "graalvm/native-image",
        label: "graalvm-native-image",
        source: "incompatible",
        note: "Oracle Linux — unsupported by Copa",
      },
      {
        name: "library/gradle",
        label: "gradle",
        source: "copa",
        upstream: "mirror.gcr.io/library/gradle",
      },
      { name: "maven", source: "integer" },
      {
        name: "bazelbuild/bazel",
        label: "bazel",
        source: "copa",
        upstream: "ghcr.io/bazelbuild/bazel",
      },
      { name: "ko", source: "integer" },
    ],
  },

  {
    id: "web",
    label: "Web Servers & Proxies",
    images: [
      { name: "caddy", source: "integer", variants: ["default", "fips"] },
      { name: "nginx", source: "integer", variants: ["default", "fips"] },
      { name: "httpd", source: "integer" },
      { name: "haproxy", source: "integer" },
      { name: "traefik", source: "integer" },
      { name: "envoy", source: "integer" },
      { name: "caddy", source: "integer", variants: ["default", "fips"] },
      {
        name: "nginxinc/nginx-s3-gateway",
        label: "nginx-s3-gateway",
        source: "copa",
        upstream: "ghcr.io/nginxinc/nginx-s3-gateway",
      },
      {
        name: "jcmoraisjr/haproxy-ingress",
        label: "haproxy-ingress",
        source: "copa",
        upstream: "quay.io/jcmoraisjr/haproxy-ingress",
      },
      {
        name: "kubernetes/ingress-nginx/controller",
        label: "ingress-nginx-controller",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/controller",
      },
      {
        name: "kubernetes/ingress-nginx/kube-webhook-certgen",
        label: "kube-webhook-certgen",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/kube-webhook-certgen",
      },
      {
        name: "kubernetes/ingress-nginx/defaultbackend",
        label: "ingress-defaultbackend",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/defaultbackend",
      },
      {
        name: "library/tomcat",
        label: "tomcat",
        source: "copa",
        upstream: "mirror.gcr.io/library/tomcat",
      },
    ],
  },

  {
    id: "databases",
    label: "Databases & Caching",
    images: [
      { name: "postgres", source: "integer", variants: ["default", "dev"] },
      {
        name: "library/redis",
        label: "redis",
        source: "copa",
        upstream: "mirror.gcr.io/library/redis",
      },
      { name: "valkey", source: "integer" },
      {
        name: "mongodb/mongodb-community-server",
        label: "mongodb",
        source: "copa",
        upstream: "docker.io/mongodb/mongodb-community-server",
      },
      { name: "mariadb", source: "integer" },
      {
        name: "library/elasticsearch",
        label: "elasticsearch",
        source: "copa",
        upstream: "mirror.gcr.io/library/elasticsearch",
      },
      {
        name: "opensearchproject/opensearch",
        label: "opensearch",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/opensearch",
      },
      {
        name: "opensearchproject/opensearch-dashboards",
        label: "opensearch-dashboards",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/opensearch-dashboards",
      },
      {
        name: "cockroachdb/cockroach",
        label: "cockroachdb",
        source: "copa",
        upstream: "mirror.gcr.io/cockroachdb/cockroach",
      },
      {
        name: "clickhouse/clickhouse-server",
        label: "clickhouse",
        source: "copa",
        upstream: "mirror.gcr.io/clickhouse/clickhouse-server",
      },
      {
        name: "rqlite/rqlite",
        label: "rqlite",
        source: "copa",
        upstream: "mirror.gcr.io/rqlite/rqlite",
      },
      { name: "influxdb", source: "integer" },
      { name: "memcached", source: "integer" },
      { name: "pgbouncer", source: "integer" },
      {
        name: "redis/redisinsight",
        label: "redisinsight",
        source: "copa",
        upstream: "mirror.gcr.io/redis/redisinsight",
      },
      {
        name: "elastic/eck-operator",
        label: "eck-operator",
        source: "copa",
        upstream: "mirror.gcr.io/elastic/eck-operator",
      },
      {
        name: "zalando/postgres-operator",
        label: "postgres-operator",
        source: "copa",
        upstream: "ghcr.io/zalando/postgres-operator",
      },
      {
        name: "opstree/redis",
        label: "Redis (OpsTree)",
        source: "copa",
        upstream: "quay.io/opstree/redis",
      },
      {
        name: "opstree/redis-sentinel",
        label: "Redis Sentinel (OpsTree)",
        source: "copa",
        upstream: "quay.io/opstree/redis-sentinel",
      },
      { name: "minio", source: "integer" },
      { name: "minio-client", source: "integer" },
    ],
  },

  {
    id: "messaging",
    label: "Messaging & Streaming",
    images: [
      { name: "rabbitmq", source: "integer" },
      {
        name: "rabbitmqoperator/cluster-operator",
        label: "rabbitmq-cluster-operator",
        source: "copa",
        upstream: "mirror.gcr.io/rabbitmqoperator/cluster-operator",
      },
      {
        name: "rabbitmqoperator/messaging-topology-operator",
        label: "rabbitmq-topology-operator",
        source: "copa",
        upstream: "mirror.gcr.io/rabbitmqoperator/messaging-topology-operator",
      },
      {
        name: "confluentinc/cp-kafka",
        label: "Kafka (Confluent)",
        source: "copa",
        upstream: "mirror.gcr.io/confluentinc/cp-kafka",
      },
      {
        name: "strimzi/kafka",
        label: "strimzi-kafka",
        source: "copa",
        upstream: "quay.io/strimzi/kafka",
      },
      { name: "nats", source: "integer" },
      { name: "kafka", source: "integer" },
      {
        name: "library/zookeeper",
        label: "zookeeper",
        source: "copa",
        upstream: "mirror.gcr.io/library/zookeeper",
      },
    ],
  },

  {
    id: "kubernetes",
    label: "Kubernetes & Orchestration",
    images: [
      { name: "kubectl", source: "integer" },
      { name: "helm", source: "integer", variants: ["default", "fips"] },
      { name: "etcd", source: "integer" },
      {
        name: "aws/karpenter",
        label: "karpenter",
        source: "copa",
        upstream: "ghcr.io/aws/karpenter",
      },
      {
        name: "kubernetes/autoscaler/cluster-autoscaler",
        label: "cluster-autoscaler",
        source: "copa",
        upstream: "ghcr.io/kubernetes/autoscaler/cluster-autoscaler",
      },
      {
        name: "kubernetes-sigs/external-dns",
        label: "external-dns",
        source: "copa",
        upstream: "ghcr.io/kubernetes-sigs/external-dns",
      },
      {
        name: "kubernetes/kube-state-metrics/kube-state-metrics",
        label: "kube-state-metrics",
        source: "integer",
        upstream: "ghcr.io/kubernetes/kube-state-metrics/kube-state-metrics",
      },
      {
        name: "emberstack/kubernetes-reflector",
        label: "kubernetes-reflector",
        source: "copa",
        upstream: "ghcr.io/emberstack/kubernetes-reflector",
      },
      {
        name: "kubernetes-sigs/secrets-store-csi-driver",
        label: "secrets-store-csi-driver",
        source: "copa",
        upstream: "ghcr.io/kubernetes-sigs/secrets-store-csi-driver",
      },
      {
        name: "googlecloudplatform/secrets-store-csi-driver-provider-gcp",
        label: "secrets-store-csi-provider-gcp",
        source: "copa",
        upstream: "ghcr.io/googlecloudplatform/secrets-store-csi-driver-provider-gcp",
      },
      {
        name: "kiwigrid/k8s-sidecar",
        label: "k8s-sidecar",
        source: "copa",
        upstream: "ghcr.io/kiwigrid/k8s-sidecar",
      },
      {
        name: "jimmidyson/configmap-reload",
        label: "configmap-reload",
        source: "copa",
        upstream: "ghcr.io/jimmidyson/configmap-reload",
      },
      {
        name: "kubernetes-sigs/node-feature-discovery",
        label: "node-feature-discovery",
        source: "copa",
        upstream: "ghcr.io/kubernetes-sigs/node-feature-discovery",
      },
      { name: "rancher/k3s", label: "k3s", source: "copa", upstream: "mirror.gcr.io/rancher/k3s" },
      {
        name: "crossplane/crossplane",
        label: "crossplane",
        source: "copa",
        upstream: "mirror.gcr.io/crossplane/crossplane",
      },
      { name: "terraform", source: "integer", variants: ["default", "fips"] },
      {
        name: "aws/eks-distro/coredns/coredns",
        label: "eks-distro-coredns",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/coredns/coredns",
      },
      {
        name: "aws/eks-distro/kubernetes/kube-apiserver",
        label: "eks-distro-kube-apiserver",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-apiserver",
      },
      {
        name: "aws/eks-distro/kubernetes/kube-scheduler",
        label: "eks-distro-kube-scheduler",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-scheduler",
      },
      {
        name: "aws/eks-distro/kubernetes/kube-proxy",
        label: "eks-distro-kube-proxy",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-proxy",
      },
      {
        name: "aws/eks-distro/kubernetes-csi/node-driver-registrar",
        label: "eks-distro-csi-node-driver-registrar",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes-csi/node-driver-registrar",
      },
    ],
  },

  {
    id: "mesh",
    label: "Service Mesh & Networking",
    images: [
      { name: "istio/proxyv2", source: "copa", upstream: "mirror.gcr.io/istio/proxyv2" },
      {
        name: "istio/pilot",
        label: "istio-pilot",
        source: "copa",
        upstream: "mirror.gcr.io/istio/pilot",
      },
      {
        name: "istio/install-cni",
        label: "istio-install-cni",
        source: "copa",
        upstream: "mirror.gcr.io/istio/install-cni",
      },
      {
        name: "cilium/cilium",
        label: "cilium-agent",
        source: "copa",
        upstream: "quay.io/cilium/cilium",
      },
      {
        name: "cilium/cilium-envoy",
        label: "cilium-envoy",
        source: "copa",
        upstream: "quay.io/cilium/cilium-envoy",
      },
      {
        name: "cilium/operator-generic",
        label: "cilium-operator",
        source: "copa",
        upstream: "quay.io/cilium/operator-generic",
      },
      {
        name: "cilium/hubble-ui",
        label: "cilium-hubble-ui",
        source: "copa",
        upstream: "quay.io/cilium/hubble-ui",
      },
      {
        name: "calico/node",
        label: "calico-node",
        source: "incompatible",
        note: "Stripped image — no package managers",
      },
      { name: "calico/cni", label: "calico-cni", source: "copa", upstream: "quay.io/calico/cni" },
      {
        name: "calico/kube-controllers",
        label: "calico-kube-controllers",
        source: "copa",
        upstream: "quay.io/calico/kube-controllers",
      },
      {
        name: "calico/typha",
        label: "calico-typha",
        source: "copa",
        upstream: "quay.io/calico/typha",
      },
      {
        name: "calico/apiserver",
        label: "calico-apiserver",
        source: "copa",
        upstream: "quay.io/calico/apiserver",
      },
      { name: "calico/csi", label: "calico-csi", source: "copa", upstream: "quay.io/calico/csi" },
      {
        name: "calico/ctl",
        label: "calico-calicoctl",
        source: "incompatible",
        note: "Stripped image — no package managers",
      },
      {
        name: "calico/node-driver-registrar",
        label: "calico-node-driver-registrar",
        source: "copa",
        upstream: "quay.io/calico/node-driver-registrar",
      },
      {
        name: "calico/key-cert-provisioner",
        label: "calico-key-cert-provisioner",
        source: "copa",
        upstream: "quay.io/calico/key-cert-provisioner",
      },
    ],
  },

  {
    id: "monitoring",
    label: "Monitoring & Observability",
    images: [
      { name: "prometheus", source: "integer" },
      { name: "alertmanager", source: "integer" },
      { name: "grafana", source: "integer" },
      { name: "loki", source: "integer" },
      { name: "mimir", source: "integer" },
      { name: "thanos", source: "integer" },
      { name: "telegraf", source: "integer" },
      { name: "vector", source: "integer" },
      { name: "pushgateway", source: "integer" },
      {
        name: "grafana/grafana-operator",
        label: "grafana-operator",
        source: "copa",
        upstream: "ghcr.io/grafana/grafana-operator",
      },
      {
        name: "prometheus/node-exporter",
        label: "prometheus-node-exporter",
        source: "copa",
        upstream: "quay.io/prometheus/node-exporter",
      },
      {
        name: "prometheus/mysqld-exporter",
        label: "prometheus-mysqld-exporter",
        source: "copa",
        upstream: "quay.io/prometheus/mysqld-exporter",
      },
      {
        name: "prometheus-operator/prometheus-config-reloader",
        label: "prometheus-config-reloader",
        source: "copa",
        upstream: "quay.io/prometheus-operator/prometheus-config-reloader",
      },
      {
        name: "prometheuscommunity/elasticsearch-exporter",
        label: "prometheus-es-exporter",
        source: "copa",
        upstream: "quay.io/prometheuscommunity/elasticsearch-exporter",
      },
      {
        name: "grafana/promtail",
        label: "promtail",
        source: "copa",
        upstream: "mirror.gcr.io/grafana/promtail",
      },
      {
        name: "datadog/agent",
        label: "datadog-agent",
        source: "copa",
        upstream: "ghcr.io/datadog/agent",
      },
      {
        name: "datadog/cluster-agent",
        label: "datadog-cluster-agent",
        source: "copa",
        upstream: "ghcr.io/datadog/cluster-agent",
      },
      {
        name: "newrelic/infrastructure-bundle",
        label: "newrelic-infra-bundle",
        source: "copa",
        upstream: "mirror.gcr.io/newrelic/infrastructure-bundle",
      },
    ],
  },

  {
    id: "logging",
    label: "Logging",
    images: [
      { name: "fluent-bit", source: "integer" },
      {
        name: "fluent/fluentd",
        label: "fluentd",
        source: "copa",
        upstream: "mirror.gcr.io/fluent/fluentd",
      },
      {
        name: "fluent/fluentd-kubernetes-daemonset",
        label: "fluentd-k8s-daemonset",
        source: "copa",
        upstream: "mirror.gcr.io/fluent/fluentd-kubernetes-daemonset",
      },
      {
        name: "fluent/fluent-operator",
        label: "fluent-operator",
        source: "copa",
        upstream: "ghcr.io/fluent/fluent-operator",
      },
      {
        name: "opensearchproject/logstash-oss-with-opensearch-output-plugin",
        label: "logstash-oss-opensearch",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/logstash-oss-with-opensearch-output-plugin",
      },
    ],
  },

  {
    id: "cicd",
    label: "CI/CD & GitOps",
    images: [
      { name: "argoproj/argocd", source: "copa", upstream: "quay.io/argoproj/argocd" },
      {
        name: "jenkins/jenkins",
        label: "jenkins",
        source: "copa",
        upstream: "mirror.gcr.io/jenkins/jenkins",
      },
      {
        name: "tektoncd/cli",
        label: "tekton-cli",
        source: "copa",
        upstream: "ghcr.io/tektoncd/cli",
      },
      {
        name: "renovatebot/renovate",
        label: "renovate",
        source: "copa",
        upstream: "ghcr.io/renovatebot/renovate",
      },
    ],
  },

  {
    id: "security",
    label: "Security & Identity",
    images: [
      { name: "vault", source: "integer" },
      {
        name: "hashicorp/vault-k8s",
        label: "vault-k8s",
        source: "copa",
        upstream: "mirror.gcr.io/hashicorp/vault-k8s",
      },
      {
        name: "hashicorp/consul",
        label: "consul",
        source: "copa",
        upstream: "mirror.gcr.io/hashicorp/consul",
      },
      {
        name: "keycloak/keycloak",
        label: "keycloak",
        source: "copa",
        upstream: "quay.io/keycloak/keycloak",
      },
      {
        name: "keycloak/keycloak-operator",
        label: "keycloak-operator",
        source: "incompatible",
        note: "Stripped image — no package managers",
      },
      {
        name: "spiffe/spire-server",
        label: "spire-server",
        source: "copa",
        upstream: "ghcr.io/spiffe/spire-server",
      },
      {
        name: "spiffe/spire-agent",
        label: "spire-agent",
        source: "copa",
        upstream: "ghcr.io/spiffe/spire-agent",
      },
      {
        name: "spiffe/spiffe-helper",
        label: "spiffe-helper",
        source: "copa",
        upstream: "ghcr.io/spiffe/spiffe-helper",
      },
      {
        name: "gravitational/teleport",
        label: "teleport",
        source: "copa",
        upstream: "quay.io/gravitational/teleport",
      },
      {
        name: "openbao/openbao",
        label: "openbao",
        source: "copa",
        upstream: "ghcr.io/openbao/openbao",
      },
      { name: "cosign", source: "integer", variants: ["default", "fips"] },
    ],
  },

  {
    id: "policy",
    label: "Policy & Compliance",
    images: [
      { name: "kyverno/kyverno", source: "copa", upstream: "ghcr.io/kyverno/kyverno" },
      {
        name: "kyverno/kyverno-cli",
        label: "kyverno-cli",
        source: "copa",
        upstream: "ghcr.io/kyverno/kyverno-cli",
      },
      {
        name: "kyverno/background-controller",
        label: "kyverno-background-controller",
        source: "copa",
        upstream: "ghcr.io/kyverno/background-controller",
      },
      {
        name: "kyverno/reports-controller",
        label: "kyverno-reports-controller",
        source: "copa",
        upstream: "ghcr.io/kyverno/reports-controller",
      },
      {
        name: "kyverno/policy-reporter-ui",
        label: "kyverno-policy-reporter-ui",
        source: "copa",
        upstream: "ghcr.io/kyverno/policy-reporter-ui",
      },
      {
        name: "openpolicyagent/gatekeeper",
        label: "gatekeeper",
        source: "copa",
        upstream: "mirror.gcr.io/openpolicyagent/gatekeeper",
      },
    ],
  },

  {
    id: "certs",
    label: "Cert Management",
    images: [
      {
        name: "jetstack/cert-manager-controller",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-controller",
      },
      {
        name: "jetstack/cert-manager-cainjector",
        label: "cert-manager-cainjector",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-cainjector",
      },
      {
        name: "jetstack/cert-manager-acmesolver",
        label: "cert-manager-acmesolver",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-acmesolver",
      },
      {
        name: "jetstack/cert-manager-cmctl",
        label: "cert-manager-cmctl",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-cmctl",
      },
      {
        name: "cert-manager/cert-manager-openshift-routes",
        label: "cert-manager-openshift-routes",
        source: "copa",
        upstream: "ghcr.io/cert-manager/cert-manager-openshift-routes",
      },
    ],
  },

  {
    id: "registries",
    label: "Container Registries",
    images: [
      {
        name: "goharbor/registry-photon",
        source: "incompatible",
        note: "Photon OS — unsupported by Copa",
      },
      {
        name: "goharbor/harbor-portal",
        source: "incompatible",
        note: "Photon OS — unsupported by Copa",
      },
      {
        name: "project-zot/zot-linux-amd64",
        label: "zot",
        source: "incompatible",
        note: "Broken Debian repos — libssl3 unpatchable",
      },
    ],
  },

  {
    id: "data",
    label: "Data & ML",
    images: [
      { name: "apache/airflow", source: "copa", upstream: "mirror.gcr.io/apache/airflow" },
      {
        name: "kubeflow/spark-operator",
        label: "spark-operator",
        source: "copa",
        upstream: "ghcr.io/kubeflow/spark-operator",
      },
      { name: "mlflow/mlflow", label: "mlflow", source: "copa", upstream: "ghcr.io/mlflow/mlflow" },
    ],
  },

  {
    id: "base",
    label: "Base & Utilities",
    images: [
      { name: "distroless/static", source: "copa", upstream: "gcr.io/distroless/static" },
      {
        name: "library/busybox",
        label: "busybox",
        source: "copa",
        upstream: "mirror.gcr.io/library/busybox",
      },
      {
        name: "library/bash",
        label: "bash",
        source: "copa",
        upstream: "mirror.gcr.io/library/bash",
      },
      { name: "curl", source: "integer" },
      { name: "git", source: "integer" },
      {
        name: "library/docker",
        label: "docker-cli",
        source: "copa",
        upstream: "mirror.gcr.io/library/docker",
      },
      {
        name: "google/cloud-sdk",
        label: "google-cloud-sdk",
        source: "copa",
        upstream: "mirror.gcr.io/google/cloud-sdk",
      },
      {
        name: "powershell/powershell",
        label: "powershell",
        source: "copa",
        upstream: "ghcr.io/powershell/powershell",
      },
      {
        name: "klakegg/hugo",
        label: "hugo",
        source: "copa",
        upstream: "mirror.gcr.io/klakegg/hugo",
      },
      {
        name: "library/wordpress",
        label: "wordpress",
        source: "copa",
        upstream: "mirror.gcr.io/library/wordpress",
      },
      {
        name: "library/sonarqube",
        label: "sonarqube",
        source: "copa",
        upstream: "mirror.gcr.io/library/sonarqube",
      },
      {
        name: "sonarsource/sonar-scanner-cli",
        label: "sonar-scanner-cli",
        source: "copa",
        upstream: "mirror.gcr.io/sonarsource/sonar-scanner-cli",
      },
      {
        name: "pendulum-project/ntpd-rs",
        label: "ntpd-rs",
        source: "copa",
        upstream: "ghcr.io/pendulum-project/ntpd-rs",
      },
      { name: "wagoodman/dive", label: "dive", source: "copa", upstream: "ghcr.io/wagoodman/dive" },
      { name: "crane", source: "integer", variants: ["default", "fips"] },
      { name: "grype", source: "integer" },
      { name: "coredns", source: "integer" },
    ],
  },
];

export const totalImages = fullCatalog.reduce((sum, cat) => sum + cat.images.length, 0);
export const totalCategories = fullCatalog.length;
export const copaCount = fullCatalog.reduce(
  (sum, cat) => sum + cat.images.filter((i) => i.source === "copa").length,
  0
);
export const integerCount = fullCatalog.reduce(
  (sum, cat) => sum + cat.images.filter((i) => i.source === "integer").length,
  0
);
