package workflowpolicy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type runsOnCatalog struct {
	Runners map[string]runsOnProfile `yaml:"runners"`
}

type runsOnProfile struct {
	CPU    int      `yaml:"cpu"`
	RAM    int      `yaml:"ram"`
	Image  string   `yaml:"image"`
	Volume string   `yaml:"volume"`
	Extras []string `yaml:"extras"`
	Spot   *bool    `yaml:"spot"`
}

func validateRunsOnCatalog(repositoryRoot string) []Violation {
	path := filepath.Join(repositoryRoot, ".github", "runs-on.yml")
	info, err := os.Lstat(path)
	if err != nil {
		return []Violation{runsOnViolation("", "read .github/runs-on.yml: "+err.Error())}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []Violation{runsOnViolation("", ".github/runs-on.yml must not be a symbolic link")}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return []Violation{runsOnViolation("", "read .github/runs-on.yml: "+err.Error())}
	}
	catalog, err := decodeRunsOnCatalog(data)
	if err != nil {
		return []Violation{runsOnViolation("", "decode .github/runs-on.yml: "+err.Error())}
	}
	if len(catalog.Runners) != 1 {
		return []Violation{runsOnViolation("", "profile catalog must contain only canary-x64 during bootstrap")}
	}
	profile, exists := catalog.Runners[runsOnProfileName]
	if !exists || !exactRunsOnCanaryProfile(&profile) {
		return []Violation{runsOnViolation("", "canary-x64 must remain on-demand, cache-free, x64, 4 CPU, 8 GiB, and 40 GiB gp3")}
	}
	return nil
}

func decodeRunsOnCatalog(data []byte) (runsOnCatalog, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var catalog runsOnCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return runsOnCatalog{}, fmt.Errorf("decode catalog: %w", err)
	}
	var trailing runsOnCatalog
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return runsOnCatalog{}, fmt.Errorf("decode trailing catalog: %w", err)
	} else if err == nil {
		return runsOnCatalog{}, errMultipleYAMLDocuments
	}
	return catalog, nil
}

func exactRunsOnCanaryProfile(profile *runsOnProfile) bool {
	return profile.CPU == 4 && profile.RAM == 8 && profile.Image == "ubuntu24-full-x64" &&
		profile.Volume == "40gb:gp3" && len(profile.Extras) == 1 && profile.Extras[0] == "otel" &&
		profile.Spot != nil && !*profile.Spot
}
