package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/config"
)

func TestValidate_rejectsGoFIPSProfileWithoutWolfiBase(t *testing.T) {
	def := &config.ImageDef{
		Name:     "caddy",
		Upstream: config.Upstream{Package: "caddy"},
		Types: map[string]config.TypeTemplate{
			"fips": {
				Base:        "wolfi-fips",
				FIPSProfile: config.FIPSProfileGo,
				Environment: map[string]string{"GODEBUG": "fips140=on"},
			},
		},
	}

	err := config.Validate(def)
	require.ErrorIs(t, err, config.ErrInvalidFIPSBase)
}
