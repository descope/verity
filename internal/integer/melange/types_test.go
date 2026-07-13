package melange

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseArchitectureNormalizesSupportedAliases(t *testing.T) {
	tests := map[string]Architecture{
		"amd64":   ArchitectureX8664,
		"x86_64":  ArchitectureX8664,
		"arm64":   ArchitectureAArch64,
		"aarch64": ArchitectureAArch64,
	}
	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			actual, err := ParseArchitecture(input)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestParseArchitectureRejectsUnsupportedValue(t *testing.T) {
	arch, err := ParseArchitecture("ppc64le")
	require.ErrorIs(t, err, errUnsupportedArchitecture)
	assert.Empty(t, arch)
}
