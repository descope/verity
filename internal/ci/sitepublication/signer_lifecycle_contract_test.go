package sitepublication

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteSigner_cancellation_zeroizes_input_without_host_key_materialization(t *testing.T) {
	// Given a valid generated sentinel key and cancellation during the final signer step.
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	key := writeSignerSentinelKeyPair(t, request)
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	runner := &recordingExecutionRunner{run: func(index int, _ ExecutionCommand) (ExecutionResult, error) {
		if index == 2 {
			cancel()
			return ExecutionResult{}, ctx.Err()
		}
		return ExecutionResult{}, nil
	}}

	// When signer execution is cancelled after preflight.
	_, err = ExecuteSigner(ctx, &plan, key, runner)

	// Then cancellation remains observable, input is zeroized, and no host key is created.
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.NoDirExists(t, plan.Cleanup.KeyDirectory)
	assert.Len(t, runner.calls, 3)
	for _, value := range key {
		assert.Zero(t, value)
	}
}

func writeSignerSentinelKeyPair(t *testing.T, request *SignerRequest) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	publicPath := filepath.Join(request.WorkspaceDir, request.PublicKeyPath)
	require.NoError(t, os.WriteFile(publicPath, publicPEM, 0o644))
	return privatePEM
}
