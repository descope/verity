//go:build integration

package integration

import (
	"path/filepath"
	"testing"
)

func TestWorkloadIdentityWebhookAllowlistAcceptsCurrentMCRPath(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Skipf("findRepoRoot failed (likely not running in repo): %v", err)
	}
	path := filepath.Join(root, "test", "chart-integration", "values", "workload-identity-webhook.allowlist.txt")
	allow, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("load workload-identity-webhook allowlist: %v", err)
	}
	image := "mcr.microsoft.com/oss/v2/azure/workload-identity/webhook:v1.6.0"
	imageID := "mcr.microsoft.com/oss/v2/azure/workload-identity/webhook@sha256:6873b8cec3fb5d585e712ea18af1c1c24107c589e4eafd20a91e260092bc8acb"
	if !isAccepted(image, imageID, allow) {
		t.Fatalf("current workload-identity-webhook MCR image must be allowlisted: image=%q imageID=%q", image, imageID)
	}
}
