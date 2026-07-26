package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"
)

const (
	apkSigningKeyEnvironment = "APK_REPOSITORY_PRIVATE_KEY"
	maxAPKSigningKeyBytes    = 64 << 10
)

var errAPKSigningKeyTooLarge = errors.New("APK signing key exceeds size limit")

func readAPKSigningKey(command *cli.Command) ([]byte, error) {
	environmentKey, fromEnvironment := os.LookupEnv(apkSigningKeyEnvironment)
	if err := os.Unsetenv(apkSigningKeyEnvironment); err != nil {
		return nil, fmt.Errorf("clear ambient APK signing key: %w", err)
	}
	var key []byte
	if fromEnvironment {
		key = []byte(environmentKey)
	} else {
		reader := command.Reader
		if reader == nil {
			reader = os.Stdin
		}
		var err error
		key, err = io.ReadAll(io.LimitReader(reader, maxAPKSigningKeyBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read APK signing key from stdin: %w", err)
		}
	}
	if len(key) > maxAPKSigningKeyBytes {
		clear(key)
		return nil, fmt.Errorf("%w: %d bytes", errAPKSigningKeyTooLarge, maxAPKSigningKeyBytes)
	}
	return key, nil
}
