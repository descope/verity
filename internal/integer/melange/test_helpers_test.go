package melange

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func testSHA(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func testPaths(root string) Paths {
	return DefaultPaths(root)
}

func testPathsPtr(root string) *Paths {
	paths := testPaths(root)
	return &paths
}

func testPath(root, relative string) string {
	return filepath.Join(root, filepath.FromSlash(relative))
}
