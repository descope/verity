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

type expectedRunsOnProfile struct {
	cpu    int
	ram    int
	image  string
	volume string
}

var expectedRunsOnProfiles = map[string]expectedRunsOnProfile{
	"canary-x64":     {cpu: 4, ram: 8, image: "ubuntu24-full-x64", volume: "40gb:gp3"},
	"ci-large-x64":   {cpu: 16, ram: 32, image: "ubuntu24-full-x64", volume: "100gb:gp3"},
	"integer-amd64":  {cpu: 32, ram: 64, image: "ubuntu24-full-x64", volume: "200gb:gp3"},
	"integer-arm64":  {cpu: 32, ram: 64, image: "ubuntu24-full-arm64", volume: "200gb:gp3"},
	"buildkit-x64":   {cpu: 16, ram: 32, image: "ubuntu24-full-x64", volume: "150gb:gp3"},
	"buildkit-arm64": {cpu: 16, ram: 32, image: "ubuntu24-full-arm64", volume: "150gb:gp3"},
	"chart-x64":      {cpu: 16, ram: 32, image: "ubuntu24-full-x64", volume: "150gb:gp3"},
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
	if len(catalog.Runners) != len(expectedRunsOnProfiles) {
		return []Violation{runsOnViolation("", "profile catalog must contain the exact reviewed capacity profiles")}
	}
	for name, expected := range expectedRunsOnProfiles {
		profile, exists := catalog.Runners[name]
		if !exists || !exactRunsOnProfile(&profile, expected) {
			return []Violation{runsOnViolation("", name+" must match its reviewed on-demand capacity profile")}
		}
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

func exactRunsOnProfile(profile *runsOnProfile, expected expectedRunsOnProfile) bool {
	return profile.CPU == expected.cpu && profile.RAM == expected.ram && profile.Image == expected.image &&
		profile.Volume == expected.volume && len(profile.Extras) == 1 && profile.Extras[0] == "otel" &&
		profile.Spot != nil && !*profile.Spot
}
