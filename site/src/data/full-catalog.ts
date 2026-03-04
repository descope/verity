export type ImageSource = "copa" | "integer";

export interface FullCatalogImage {
  name: string;
  source: ImageSource;
  upstream?: string;
  variants?: string[];
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
      { name: "graalvm-native-image", source: "copa", upstream: "ghcr.io/graalvm/native-image" },
      { name: "gradle", source: "copa", upstream: "mirror.gcr.io/library/gradle" },
      { name: "maven", source: "integer" },
      { name: "bazel", source: "copa", upstream: "ghcr.io/bazelbuild/bazel" },
      { name: "ko", source: "integer" },
    ],
  },

  {
    id: "web",
    label: "Web Servers & Proxies",
    images: [
      { name: "nginx", source: "integer", variants: ["default", "fips"] },
      { name: "httpd", source: "integer" },
      { name: "haproxy", source: "integer" },
      { name: "traefik", source: "integer" },
      { name: "envoy", source: "integer" },
      { name: "nginx-s3-gateway", source: "copa", upstream: "ghcr.io/nginxinc/nginx-s3-gateway" },
      { name: "haproxy-ingress", source: "copa", upstream: "quay.io/jcmoraisjr/haproxy-ingress" },
      {
        name: "ingress-nginx-controller",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/controller",
      },
      {
        name: "kube-webhook-certgen",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/kube-webhook-certgen",
      },
      {
        name: "ingress-defaultbackend",
        source: "copa",
        upstream: "ghcr.io/kubernetes/ingress-nginx/defaultbackend",
      },
      { name: "tomcat", source: "copa", upstream: "mirror.gcr.io/library/tomcat" },
    ],
  },

  {
    id: "databases",
    label: "Databases & Caching",
    images: [
      { name: "postgres", source: "integer", variants: ["default", "dev"] },
      { name: "redis", source: "copa", upstream: "mirror.gcr.io/library/redis" },
      { name: "valkey", source: "integer" },
      { name: "mongodb", source: "copa", upstream: "mirror.gcr.io/library/mongo" },
      { name: "mariadb", source: "integer" },
      { name: "elasticsearch", source: "copa", upstream: "mirror.gcr.io/library/elasticsearch" },
      {
        name: "opensearch",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/opensearch",
      },
      {
        name: "opensearch-dashboards",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/opensearch-dashboards",
      },
      { name: "cockroachdb", source: "copa", upstream: "mirror.gcr.io/cockroachdb/cockroach" },
      {
        name: "clickhouse",
        source: "copa",
        upstream: "mirror.gcr.io/clickhouse/clickhouse-server",
      },
      { name: "rqlite", source: "copa", upstream: "mirror.gcr.io/rqlite/rqlite" },
      { name: "influxdb", source: "integer" },
      { name: "memcached", source: "integer" },
      { name: "pgbouncer", source: "integer" },
      { name: "redisinsight", source: "copa", upstream: "mirror.gcr.io/redis/redisinsight" },
      { name: "eck-operator", source: "copa", upstream: "mirror.gcr.io/elastic/eck-operator" },
      { name: "redis (opstree)", source: "copa", upstream: "quay.io/opstree/redis" },
      {
        name: "redis-sentinel (opstree)",
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
        name: "rabbitmq-cluster-operator",
        source: "copa",
        upstream: "mirror.gcr.io/rabbitmqoperator/cluster-operator",
      },
      {
        name: "rabbitmq-topology-operator",
        source: "copa",
        upstream: "mirror.gcr.io/rabbitmqoperator/messaging-topology-operator",
      },
      {
        name: "kafka (confluent)",
        source: "copa",
        upstream: "mirror.gcr.io/confluentinc/cp-kafka",
      },
      { name: "strimzi-kafka", source: "copa", upstream: "quay.io/strimzi/kafka" },
      { name: "nats", source: "integer" },
      { name: "zookeeper", source: "copa", upstream: "mirror.gcr.io/library/zookeeper" },
    ],
  },

  {
    id: "kubernetes",
    label: "Kubernetes & Orchestration",
    images: [
      { name: "kubectl", source: "integer" },
      { name: "helm", source: "integer" },
      { name: "etcd", source: "integer" },
      { name: "karpenter", source: "copa", upstream: "ghcr.io/aws/karpenter" },
      {
        name: "cluster-autoscaler",
        source: "copa",
        upstream: "ghcr.io/kubernetes/autoscaler/cluster-autoscaler",
      },
      { name: "external-dns", source: "copa", upstream: "ghcr.io/kubernetes-sigs/external-dns" },
      {
        name: "kube-state-metrics",
        source: "copa",
        upstream: "ghcr.io/kubernetes/kube-state-metrics/kube-state-metrics",
      },
      {
        name: "kubernetes-reflector",
        source: "copa",
        upstream: "ghcr.io/emberstack/kubernetes-reflector",
      },
      {
        name: "secrets-store-csi-driver",
        source: "copa",
        upstream: "ghcr.io/kubernetes-sigs/secrets-store-csi-driver",
      },
      {
        name: "secrets-store-csi-provider-gcp",
        source: "copa",
        upstream: "ghcr.io/googlecloudplatform/secrets-store-csi-driver-provider-gcp",
      },
      { name: "k8s-sidecar", source: "copa", upstream: "ghcr.io/kiwigrid/k8s-sidecar" },
      { name: "configmap-reload", source: "copa", upstream: "ghcr.io/jimmidyson/configmap-reload" },
      {
        name: "node-feature-discovery",
        source: "copa",
        upstream: "ghcr.io/kubernetes-sigs/node-feature-discovery",
      },
      { name: "k3s", source: "copa", upstream: "mirror.gcr.io/rancher/k3s" },
      { name: "crossplane", source: "copa", upstream: "mirror.gcr.io/crossplane/crossplane" },
      { name: "terraform", source: "integer" },
      {
        name: "eks-distro-coredns",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/coredns/coredns",
      },
      {
        name: "eks-distro-kube-apiserver",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-apiserver",
      },
      {
        name: "eks-distro-kube-scheduler",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-scheduler",
      },
      {
        name: "eks-distro-kube-proxy",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes/kube-proxy",
      },
      {
        name: "eks-distro-csi-node-driver-registrar",
        source: "copa",
        upstream: "ghcr.io/aws/eks-distro/kubernetes-csi/node-driver-registrar",
      },
    ],
  },

  {
    id: "mesh",
    label: "Service Mesh & Networking",
    images: [
      { name: "istio-proxy", source: "copa", upstream: "mirror.gcr.io/istio/proxyv2" },
      { name: "istio-pilot", source: "copa", upstream: "mirror.gcr.io/istio/pilot" },
      { name: "istio-install-cni", source: "copa", upstream: "mirror.gcr.io/istio/install-cni" },
      { name: "cilium-agent", source: "copa", upstream: "quay.io/cilium/cilium" },
      { name: "cilium-envoy", source: "copa", upstream: "quay.io/cilium/cilium-envoy" },
      { name: "cilium-operator", source: "copa", upstream: "quay.io/cilium/operator-generic" },
      { name: "cilium-hubble-ui", source: "copa", upstream: "quay.io/cilium/hubble-ui" },
      { name: "calico-node", source: "copa", upstream: "quay.io/calico/node" },
      { name: "calico-cni", source: "copa", upstream: "quay.io/calico/cni" },
      {
        name: "calico-kube-controllers",
        source: "copa",
        upstream: "quay.io/calico/kube-controllers",
      },
      { name: "calico-typha", source: "copa", upstream: "quay.io/calico/typha" },
      { name: "calico-apiserver", source: "copa", upstream: "quay.io/calico/apiserver" },
      { name: "calico-csi", source: "copa", upstream: "quay.io/calico/csi" },
      { name: "calico-calicoctl", source: "copa", upstream: "quay.io/calico/ctl" },
      {
        name: "calico-node-driver-registrar",
        source: "copa",
        upstream: "quay.io/calico/node-driver-registrar",
      },
      {
        name: "calico-key-cert-provisioner",
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
      { name: "grafana-operator", source: "copa", upstream: "ghcr.io/grafana/grafana-operator" },
      {
        name: "prometheus-node-exporter",
        source: "copa",
        upstream: "quay.io/prometheus/node-exporter",
      },
      {
        name: "prometheus-mysqld-exporter",
        source: "copa",
        upstream: "quay.io/prometheus/mysqld-exporter",
      },
      {
        name: "prometheus-es-exporter",
        source: "copa",
        upstream: "quay.io/prometheuscommunity/elasticsearch-exporter",
      },
      { name: "promtail", source: "copa", upstream: "mirror.gcr.io/grafana/promtail" },
      { name: "datadog-agent", source: "copa", upstream: "ghcr.io/datadog/agent" },
      { name: "datadog-cluster-agent", source: "copa", upstream: "ghcr.io/datadog/cluster-agent" },
      {
        name: "newrelic-infra-bundle",
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
      { name: "fluentd", source: "copa", upstream: "mirror.gcr.io/fluent/fluentd" },
      {
        name: "fluentd-k8s-daemonset",
        source: "copa",
        upstream: "mirror.gcr.io/fluent/fluentd-kubernetes-daemonset",
      },
      { name: "fluent-operator", source: "copa", upstream: "ghcr.io/fluent/fluent-operator" },
      {
        name: "logstash-oss-opensearch",
        source: "copa",
        upstream: "mirror.gcr.io/opensearchproject/logstash-oss-with-opensearch-output-plugin",
      },
    ],
  },

  {
    id: "cicd",
    label: "CI/CD & GitOps",
    images: [
      { name: "argocd", source: "copa", upstream: "quay.io/argoproj/argocd" },
      { name: "jenkins", source: "copa", upstream: "mirror.gcr.io/jenkins/jenkins" },
      { name: "tekton-cli", source: "copa", upstream: "ghcr.io/tektoncd/cli" },
      { name: "renovate", source: "copa", upstream: "ghcr.io/renovatebot/renovate" },
    ],
  },

  {
    id: "security",
    label: "Security & Identity",
    images: [
      { name: "vault", source: "integer" },
      { name: "vault-k8s", source: "copa", upstream: "mirror.gcr.io/hashicorp/vault-k8s" },
      { name: "consul", source: "copa", upstream: "mirror.gcr.io/hashicorp/consul" },
      { name: "keycloak", source: "copa", upstream: "quay.io/keycloak/keycloak" },
      { name: "keycloak-operator", source: "copa", upstream: "quay.io/keycloak/keycloak-operator" },
      { name: "spire-server", source: "copa", upstream: "ghcr.io/spiffe/spire-server" },
      { name: "spire-agent", source: "copa", upstream: "ghcr.io/spiffe/spire-agent" },
      { name: "spiffe-helper", source: "copa", upstream: "ghcr.io/spiffe/spiffe-helper" },
      { name: "teleport", source: "copa", upstream: "quay.io/gravitational/teleport" },
      { name: "openbao", source: "copa", upstream: "ghcr.io/openbao/openbao" },
      { name: "cosign", source: "integer" },
    ],
  },

  {
    id: "policy",
    label: "Policy & Compliance",
    images: [
      { name: "kyverno", source: "copa", upstream: "ghcr.io/kyverno/kyverno" },
      { name: "kyverno-cli", source: "copa", upstream: "ghcr.io/kyverno/kyverno-cli" },
      {
        name: "kyverno-background-controller",
        source: "copa",
        upstream: "ghcr.io/kyverno/background-controller",
      },
      {
        name: "kyverno-reports-controller",
        source: "copa",
        upstream: "ghcr.io/kyverno/reports-controller",
      },
      {
        name: "kyverno-policy-reporter-ui",
        source: "copa",
        upstream: "ghcr.io/kyverno/policy-reporter-ui",
      },
      { name: "gatekeeper", source: "copa", upstream: "mirror.gcr.io/openpolicyagent/gatekeeper" },
    ],
  },

  {
    id: "certs",
    label: "Cert Management",
    images: [
      {
        name: "cert-manager-controller",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-controller",
      },
      {
        name: "cert-manager-cainjector",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-cainjector",
      },
      {
        name: "cert-manager-acmesolver",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-acmesolver",
      },
      {
        name: "cert-manager-cmctl",
        source: "copa",
        upstream: "quay.io/jetstack/cert-manager-cmctl",
      },
      {
        name: "cert-manager-openshift-routes",
        source: "copa",
        upstream: "ghcr.io/cert-manager/cert-manager-openshift-routes",
      },
    ],
  },

  {
    id: "registries",
    label: "Container Registries",
    images: [
      { name: "harbor-registry", source: "copa", upstream: "ghcr.io/goharbor/registry-photon" },
      { name: "harbor-portal", source: "copa", upstream: "ghcr.io/goharbor/harbor-portal" },
      { name: "zot", source: "copa", upstream: "ghcr.io/project-zot/zot-linux-amd64" },
    ],
  },

  {
    id: "data",
    label: "Data & ML",
    images: [
      { name: "airflow", source: "copa", upstream: "mirror.gcr.io/apache/airflow" },
      { name: "spark-operator", source: "copa", upstream: "ghcr.io/kubeflow/spark-operator" },
      { name: "mlflow", source: "copa", upstream: "ghcr.io/mlflow/mlflow" },
    ],
  },

  {
    id: "base",
    label: "Base & Utilities",
    images: [
      { name: "static-distroless", source: "copa", upstream: "gcr.io/distroless/static" },
      { name: "busybox", source: "copa", upstream: "mirror.gcr.io/library/busybox" },
      { name: "bash", source: "copa", upstream: "mirror.gcr.io/library/bash" },
      { name: "curl", source: "integer" },
      { name: "git", source: "integer" },
      { name: "docker-cli", source: "copa", upstream: "mirror.gcr.io/library/docker" },
      { name: "google-cloud-sdk", source: "copa", upstream: "mirror.gcr.io/google/cloud-sdk" },
      { name: "powershell", source: "copa", upstream: "ghcr.io/powershell/powershell" },
      { name: "hugo", source: "copa", upstream: "mirror.gcr.io/klakegg/hugo" },
      { name: "wordpress", source: "copa", upstream: "mirror.gcr.io/library/wordpress" },
      { name: "sonarqube", source: "copa", upstream: "mirror.gcr.io/library/sonarqube" },
      {
        name: "sonar-scanner-cli",
        source: "copa",
        upstream: "mirror.gcr.io/sonarsource/sonar-scanner-cli",
      },
      { name: "ntpd-rs", source: "copa", upstream: "ghcr.io/pendulum-project/ntpd-rs" },
      { name: "dive", source: "copa", upstream: "ghcr.io/wagoodman/dive" },
      { name: "crane", source: "integer" },
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
