package cmd

import (
	"fmt"

	"github.com/verity-org/verity/internal/ci/publication"
)

func resolvePublicationSigningKeyState(
	path, repositoryDir string,
	epoch uint64,
	active string,
	trusted, revoked []string,
) (publication.SigningKeyState, error) {
	if path == "" {
		return publication.SigningKeyState{
			Epoch: epoch, ActiveFingerprint: active,
			TrustedFingerprints: trusted, RevokedFingerprints: revoked,
		}, nil
	}
	if epoch != 0 || active != "" || len(trusted) != 0 || len(revoked) != 0 {
		return publication.SigningKeyState{}, fmt.Errorf(
			"%w: --signing-key-state is mutually exclusive with raw signing key fields",
			errInvalidPublicationArguments,
		)
	}
	state, err := publication.LoadSigningKeyState(path, repositoryDir)
	if err != nil {
		return publication.SigningKeyState{}, err
	}
	return state, nil
}
