package apkindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPackageNameStripsRepositoryTagsAndVersionConstraints(t *testing.T) {
	tests := map[string]string{
		"crane":                         "crane",
		"crane=0.21.7-r1":               "crane",
		"crane@local=0.21.7-r1":         "crane",
		"tempo~2.10.0":                  "tempo",
		"linkerd2-cli>=25.12.3-r100":    "linkerd2-cli",
		"cilium-1.19@local<1.19.6-r0":   "cilium-1.19",
		"postgresql-18-client!=18.1-r0": "postgresql-18-client",
	}

	for packageSpec, expected := range tests {
		t.Run(packageSpec, func(t *testing.T) {
			assert.Equal(t, expected, PackageName(packageSpec))
		})
	}
}
