package melange

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

var (
	imagePattern      = regexp.MustCompile(`^[a-z][a-z0-9-]*(/[a-z][a-z0-9-]*)*$`)
	typePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	versionPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

func ResolveSpec(imagesDir, image, version, imageType string) (Spec, error) {
	if !imagePattern.MatchString(image) {
		return Spec{}, fmt.Errorf("%w %q", errInvalidImage, image)
	}
	if !typePattern.MatchString(imageType) {
		return Spec{}, fmt.Errorf("%w %q", errInvalidImageType, imageType)
	}
	if !versionPattern.MatchString(version) {
		return Spec{}, fmt.Errorf("%w %q", errInvalidVersion, version)
	}

	def, err := intconfig.LoadImageByName(imagesDir, image)
	if err != nil {
		return Spec{}, fmt.Errorf("load image %q: %w", image, err)
	}
	_, ok := def.Types[imageType]
	if !ok {
		return Spec{}, fmt.Errorf("%w: image %q type %q", errImageTypeNotFound, image, imageType)
	}
	configSpec := def.MelangeFor(version, imageType)
	if configSpec == nil {
		return Spec{}, nil
	}
	if meta, ok := def.Versions[version]; len(def.Versions) > 0 && !ok {
		return Spec{}, fmt.Errorf("%w: image %q version %q", errImageVersionNotFound, image, version)
	} else if ok && slices.Contains(meta.SkipTypes, imageType) {
		return Spec{}, fmt.Errorf("%w: image %q version %q type %q", errImageTypeSkipped, image, version, imageType)
	}
	return ResolveConfigSpec(configSpec, version)
}

func ResolveConfigSpec(input *intconfig.MelangeSpec, version string) (Spec, error) {
	if input == nil {
		return Spec{}, nil
	}
	spec := Spec{
		Upstream:    replaceVersion(input.Upstream, version),
		EnvFile:     replaceVersion(input.EnvFile, version),
		BuildOption: replaceVersion(input.BuildOption, version),
	}
	if len(input.Bespoke) > 0 {
		spec.Bespoke = make([]string, len(input.Bespoke))
	}
	for i, file := range input.Bespoke {
		spec.Bespoke[i] = replaceVersion(file, version)
	}
	if (spec.Upstream == "") == (len(spec.Bespoke) == 0) {
		return Spec{}, errInvalidSpecSource
	}
	if spec.Upstream != "" && !validIdentifier(spec.Upstream) {
		return Spec{}, fmt.Errorf("%w %q", errInvalidUpstreamKey, spec.Upstream)
	}
	for _, file := range spec.Bespoke {
		if !validIdentifier(file) {
			return Spec{}, fmt.Errorf("%w %q", errInvalidBespokeFilename, file)
		}
	}
	for label, value := range map[string]string{"env file": spec.EnvFile, "build option": spec.BuildOption} {
		if value != "" && !validIdentifier(value) {
			return Spec{}, fmt.Errorf("%w %s %q", errInvalidOptionalField, label, value)
		}
	}
	return spec, nil
}

func WriteGitHubOutput(w io.Writer, spec Spec) error {
	_, err := fmt.Fprintf(w, "needed=%t\nenv_file=%s\nbuild_option=%s\n", spec.Needed(), spec.EnvFile, spec.BuildOption)
	return err
}

func replaceVersion(value, version string) string {
	return strings.ReplaceAll(value, "{{version}}", version)
}

func validIdentifier(value string) bool {
	return value != "." && value != ".." && identifierPattern.MatchString(value)
}
