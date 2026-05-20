//go:build integration

package integration

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestWorkloadIdentityWebhookCertSecretManifestSeedsWellFormedSecret(t *testing.T) {
	manifest, err := workloadIdentityWebhookCertSecretManifest("workload-identity-webhook", time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatalf("workloadIdentityWebhookCertSecretManifest: %v", err)
	}
	for _, want := range []string{
		"kind: Namespace",
		"name: workload-identity-webhook",
		"kind: Secret",
		"name: azure-wi-webhook-server-cert",
		"type: kubernetes.io/tls",
		"  ca.crt:",
		"  tls.crt:",
		"  tls.key:",
	} {
		if !strings.Contains(manifest, want) {
			t.Fatalf("manifest missing %q:\n%s", want, manifest)
		}
	}

	for _, key := range []string{"ca.crt", "tls.crt", "tls.key"} {
		re := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(key) + `: (\S+)$`)
		match := re.FindStringSubmatch(manifest)
		if len(match) != 2 {
			t.Fatalf("manifest missing data.%s:\n%s", key, manifest)
		}
		if _, err := base64.StdEncoding.DecodeString(match[1]); err != nil {
			t.Fatalf("data.%s is not base64: %v", key, err)
		}
	}
}

func TestWorkloadIdentityWebhookUsesTakeOwnershipForSeededSecret(t *testing.T) {
	if !chartTakeOwnership["workload-identity-webhook"] {
		t.Fatalf("workload-identity-webhook must use --take-ownership to adopt seeded server-cert Secret")
	}
}
