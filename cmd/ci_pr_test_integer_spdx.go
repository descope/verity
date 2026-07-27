package cmd

import (
	"encoding/json"
	"fmt"
	"os"
)

const prMaxSPDXBytes = 16 << 20

type prSPDXDocument struct {
	SPDXVersion string          `json:"spdxVersion"`
	Packages    []prSPDXPackage `json:"packages"`
}

type prSPDXPackage struct {
	Name            string `json:"name"`
	VersionInfo     string `json:"versionInfo"`
	LicenseDeclared string `json:"licenseDeclared"`
}

func verifyPRSPDXPackage(path, name, version, license string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > prMaxSPDXBytes {
		return fmt.Errorf("%w: SPDX document %q is missing, non-regular, or oversized", errPRCommandFailed, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SPDX document: %w", err)
	}
	var document prSPDXDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse SPDX document: %w", err)
	}
	if document.SPDXVersion != "SPDX-2.3" {
		return fmt.Errorf("%w: SPDX version is %q", errPRCommandFailed, document.SPDXVersion)
	}
	for _, pkg := range document.Packages {
		if pkg.Name == name && pkg.VersionInfo == version && pkg.LicenseDeclared == license {
			return nil
		}
	}
	return fmt.Errorf("%w: SPDX package %s@%s with license %s is missing", errPRCommandFailed, name, version, license)
}
