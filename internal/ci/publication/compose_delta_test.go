package publication

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	ci "github.com/verity-org/verity/internal/ci"
)

func TestOperationsFromDelta_links_exact_Integer_artifact(t *testing.T) {
	// Given an exact Integer aggregate and an exact current APK delta operation.
	integerData := composeIntegerManifest(t)
	integer, err := ci.ParseIntegerBatchManifest(integerData)
	require.NoError(t, err)
	delta := []byte(`{"format_version":1,"base_sha256":"sha256:` + strings.Repeat("6", 64) + `","repository_format":"apk-v1","key_sha256":"sha256:` + strings.Repeat("7", 64) + `","operations":[{"action":"upsert","architecture":"x86_64","package":"demo","source":"x86_64/demo.apk","sha256":"` + string(digestSeed("4")) + `"}]}`)

	// When the delta is materialized into publication operations.
	operations, err := operationsFromDelta(delta, &integer)

	// Then the package is bound to the exact Integer artifact name and digest.
	require.NoError(t, err)
	require.Equal(t, []APKOperation{{
		Action: APKUpsert, Architecture: ArchitectureX8664, PackageName: "demo",
		ArtifactName: "apk-repository-build-site-42-3-1", ArtifactDigest: digestSeed("3"),
	}}, operations)
}

func TestOperationsFromDelta_rejects_duplicate_keys_and_undeclared_packages(t *testing.T) {
	integerData := composeIntegerManifest(t)
	integer, err := ci.ParseIntegerBatchManifest(integerData)
	require.NoError(t, err)
	base := `{"format_version":1,"base_sha256":"sha256:` + strings.Repeat("6", 64) + `","repository_format":"apk-v1","key_sha256":"sha256:` + strings.Repeat("7", 64) + `","operations":%s}`
	operations := []string{
		`[{"action":"remove","action":"upsert","architecture":"x86_64","package":"demo"}]`,
		`[{"action":"upsert","architecture":"x86_64","package":"attacker","source":"x86_64/attacker.apk","sha256":"` + string(digestSeed("8")) + `"}]`,
	}
	for _, value := range operations {
		data := strings.Replace(base, "%s", value, 1)
		_, err := operationsFromDelta([]byte(data), &integer)
		require.Error(t, err)
	}
}
