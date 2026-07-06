package cmd

import (
	"fmt"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func integerResolveLatestVersion(def *intconfig.ImageDef, apkindexURL string) (string, error) {
	pkgs, err := apkindex.Fetch(apkindexURL, "", 0)
	if err != nil {
		return "", fmt.Errorf("fetching APKINDEX: %w", err)
	}
	return integerResolveLatestVersionFromPkgs(def, pkgs)
}
