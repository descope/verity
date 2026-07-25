package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordingPRIntegerRunner struct {
	calls []prCommandRequest
}

func (r *recordingPRIntegerRunner) Run(_ context.Context, request *prCommandRequest) (prCommandResult, error) {
	r.calls = append(r.calls, *request)
	if request.Name == "docker" {
		if len(request.Args) > 0 && request.Args[0] == "load" {
			return prCommandResult{Stdout: []byte("Loaded image: local/demo:mutable\n")}, nil
		}
		if len(request.Args) > 1 && request.Args[0] == "image" && request.Args[1] == "inspect" {
			return prCommandResult{Stdout: []byte(`[{"Id":"sha256:` + strings.Repeat("a", 64) + `","Architecture":"arm64","Config":{"User":"65532"}}]`)}, nil
		}
	}
	return prCommandResult{}, nil
}

func TestLoadPRIntegerImage_returns_content_addressed_runtime_reference(t *testing.T) {
	// Given: docker load reports a mutable tag while inspect reports the immutable image ID.
	runner := &recordingPRIntegerRunner{}

	// When: the native image is loaded and checked.
	loaded, err := loadPRIntegerImage(t.Context(), runner, prIntegerLoadRequest{
		TarPath: "image.tar", Architecture: "arm64",
	})

	// Then: every later runtime check can use the digest-bound image ID.
	require.NoError(t, err)
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), loaded.Reference)
	require.Equal(t, "arm64", loaded.Architecture)
	require.Equal(t, "65532", loaded.User)
	require.Len(t, runner.calls, 2)
	require.Equal(t, []string{"image", "inspect", "local/demo:mutable"}, runner.calls[1].Args)
}

func TestLoadPRIntegerImage_rejects_runtime_architecture_mismatch(t *testing.T) {
	// Given: a loaded image whose immutable metadata is not native to the matrix leg.
	runner := &recordingPRIntegerRunner{}
	// When: the amd64 leg receives the arm64 image.
	_, err := loadPRIntegerImage(t.Context(), runner, prIntegerLoadRequest{
		TarPath: "image.tar", Architecture: "amd64",
	})

	// Then: execution fails before the image can be run.
	require.ErrorIs(t, err, errPRCommandFailed)
}
