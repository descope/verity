package chartgen

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

type ValueOverride struct {
	Path       string `json:"path"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Value      string `json:"value,omitempty"`
	// IsScalarOverride distinguishes a scalar-write override (the leaf
	// node at `Path` becomes the literal string `Value`, even when
	// `Value == ""`) from the legacy "if Value != \"\" then write" rule.
	// Used by the global-registry neutralisation path (#308 wave 3) to
	// emit `<scope>.global.imageRegistry: ""` and similar overrides that
	// must be honoured even when the value is an empty string. Without
	// this flag the empty-string scalar would silently disappear,
	// leaving the upstream's `global.imageRegistry: docker.io` default
	// in place and triggering the wave-3 double-registry render bug.
	IsScalarOverride bool `json:"isScalarOverride,omitempty"`
	// SetRegistry, when non-empty, instructs the wrapper renderer to
	// write `<path>.registry: <SetRegistry>` and to render Repository
	// with the registry hostname stripped. The chart's template
	// `{{ <path>.registry }}/{{ <path>.repository }}` therefore composes
	// to `verity.supply/<repo>:<tag>` whether the template
	// short-circuits via `default` or concatenates directly:
	//
	//   {{ .Values.image.registry }}/{{ repository }}             → verity.supply/<repo>
	//   {{ registry | default defaultRegistry }}/{{ repository }} → verity.supply/<repo> (registry fires)
	//
	// IMPORTANT scoping caveat: SetRegistry only writes the per-image
	// registry sibling at `<path>.registry`. If the chart's template
	// further defers to a `global.imageRegistry` (Bitnami) or
	// `global.image.registry` (Grafana / tempo-distributed) AND that
	// global field is non-empty upstream, Helm renders the global
	// hostname in front of our stripped repository →
	// `docker.io/verity-org/<repo>:<tag>`. `resolveValuePathPairs`
	// therefore also emits `IsScalarOverride` entries for any
	// global-registry sibling in the same scope (root or subchart),
	// neutralising them to empty so the per-image SetRegistry wins.
	// See `collectGlobalRegistryPaths` for the detection rule and the
	// wave-3 regression coverage in TestComposeRegistryRendersValidImageRefs
	// (`global.image.registry takes precedence over image.registry`).
	//
	// SetRegistry replaces the prior "ClearRegistry: bool" plumbing,
	// which wrote `registry: ""` and consequently produced leading-slash
	// renders (`/ghcr.io/...`) for any chart whose template did not
	// `default` to a non-empty value (issue #308 wave 2). Empty string
	// means "leave the registry sibling untouched" — same behaviour as
	// the previous `ClearRegistry: false`.
	SetRegistry string `json:"setRegistry,omitempty"`
	// SetDefaultRegistry is the parallel knob for the `defaultRegistry`
	// sibling (kyverno 3.7.x convention; postgres-operator/jenkins/etc.
	// also adopted it). Same compose-correctly semantics as SetRegistry,
	// applied to a different field.
	SetDefaultRegistry string `json:"setDefaultRegistry,omitempty"`
}

type repoTagPair struct {
	Path               string
	Repo               string
	HasTag             bool
	Registry           string
	HasRegistry        bool
	DefaultRegistry    string
	HasDefaultRegistry bool
}

func ResolveValuePaths(valuesYAML []byte, mappings []ImageMapping, overrides map[string]config.Override) ([]ValueOverride, error) {
	return ResolveValuePathsWithSubcharts(valuesYAML, nil, mappings, overrides)
}

func ResolveValuePathsWithSubcharts(valuesYAML []byte, subchartValues map[string][]byte, mappings []ImageMapping, overrides map[string]config.Override) ([]ValueOverride, error) {
	pairs, err := collectValuePairs(valuesYAML, "")
	if err != nil {
		return nil, fmt.Errorf("collect parent chart values: %w", err)
	}
	globals, err := collectGlobalRegistryPaths(valuesYAML, "")
	if err != nil {
		return nil, fmt.Errorf("collect parent chart global registries: %w", err)
	}

	subchartNames := make([]string, 0, len(subchartValues))
	for name := range subchartValues {
		subchartNames = append(subchartNames, name)
	}
	sort.Strings(subchartNames)
	for _, name := range subchartNames {
		subchartPairs, err := collectValuePairs(subchartValues[name], name)
		if err != nil {
			return nil, fmt.Errorf("collect subchart values for %q: %w", name, err)
		}
		pairs = append(pairs, subchartPairs...)
		subchartGlobals, err := collectGlobalRegistryPaths(subchartValues[name], name)
		if err != nil {
			return nil, fmt.Errorf("collect global registries for subchart %q: %w", name, err)
		}
		globals = append(globals, subchartGlobals...)
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Path < pairs[j].Path
	})
	sort.Strings(globals)

	return resolveValuePathPairs(pairs, globals, mappings, overrides), nil
}

func resolveValuePathPairs(pairs []repoTagPair, globalRegistryPaths []string, mappings []ImageMapping, overrides map[string]config.Override) []ValueOverride {
	result := make([]ValueOverride, 0, len(mappings))
	matched := make([]bool, len(mappings))

	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)

	pathHasRegistry := make(map[string]bool, len(pairs))
	pathHasDefaultRegistry := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		if p.HasRegistry {
			pathHasRegistry[p.Path] = true
		}
		if p.HasDefaultRegistry {
			pathHasDefaultRegistry[p.Path] = true
		}
	}

	for i, m := range mappings {
		for _, key := range overrideKeys {
			override := overrides[key]
			if override.ValuePath == "" {
				continue
			}
			if matchesRepo(m.OriginalRepo, key) {
				result = append(result, buildValueOverride(
					override.ValuePath,
					m.PatchedRepo,
					m.PatchedTag,
					pathHasRegistry[override.ValuePath],
					pathHasDefaultRegistry[override.ValuePath],
				))
				matched[i] = true
				break
			}
		}
	}

	usedPairs := make(map[int]bool, len(mappings))
	for i, m := range mappings {
		if matched[i] {
			continue
		}
		best, bestIdx := bestRepoTagPair(pairs, m.OriginalRepo, usedPairs)
		if best == nil {
			continue
		}
		result = append(result, buildValueOverride(
			best.Path,
			m.PatchedRepo,
			m.PatchedTag,
			best.HasRegistry,
			best.HasDefaultRegistry,
		))
		matched[i] = true
		usedPairs[bestIdx] = true
	}

	return appendGlobalRegistryNeutralisations(result, globalRegistryPaths)
}

func bestRepoTagPair(pairs []repoTagPair, originalRepo string, used map[int]bool) (best *repoTagPair, bestIdx int) {
	bestIdx = -1
	for idx := range pairs {
		if used[idx] {
			continue
		}
		pair := &pairs[idx]
		if !pair.HasTag || !matchesRepo(pair.Repo, originalRepo) {
			continue
		}
		if best == nil || preferRepoTagPair(pair, best) {
			best = pair
			bestIdx = idx
		}
	}
	return best, bestIdx
}

func preferRepoTagPair(candidate, current *repoTagPair) bool {
	candidateHasRegistry := candidate.HasRegistry || candidate.HasDefaultRegistry
	currentHasRegistry := current.HasRegistry || current.HasDefaultRegistry
	if candidateHasRegistry != currentHasRegistry {
		return candidateHasRegistry
	}
	if candidateDepth, currentDepth := pathDepth(candidate.Path), pathDepth(current.Path); candidateDepth != currentDepth {
		return candidateDepth > currentDepth
	}
	return len(candidate.Path) > len(current.Path)
}

func pathDepth(path string) int {
	if path == "" {
		return 0
	}
	return strings.Count(path, ".") + 1
}

// appendGlobalRegistryNeutralisations appends an IsScalarOverride entry
// for every global-registry path whose scope matches one of the
// per-image overrides already in `result`. See #308 wave 3: without this
// step a chart whose template defers to `global.imageRegistry` (Bitnami)
// or `global.image.registry` (tempo-distributed, several Grafana
// sub-charts) would render `<upstream-global>/verity-org/<repo>:<tag>`
// because the global hostname takes precedence over our per-image
// SetRegistry. Empty global → Bitnami's `if .global.imageRegistry` is
// false (falls through to per-image) and tempo's `coalesce` skips it.
func appendGlobalRegistryNeutralisations(result []ValueOverride, globalRegistryPaths []string) []ValueOverride {
	scopesToNeutralise := globalNeutralisationScopes(result)
	for _, globalPath := range globalRegistryPaths {
		if !scopesToNeutralise[scopeOfPath(globalPath)] {
			continue
		}
		result = append(result, ValueOverride{
			Path:             globalPath,
			IsScalarOverride: true,
			Value:            "",
		})
	}
	return result
}

// globalNeutralisationScopes returns the set of value-path scopes
// (subchart prefix or "" for root) where at least one image-override
// rewrote a per-image registry sibling, signalling that any sibling
// `global.<...>Registry` field in that scope must be neutralised so the
// chart's template doesn't prefer the upstream global over our rewrite.
func globalNeutralisationScopes(overrides []ValueOverride) map[string]bool {
	scopes := make(map[string]bool)
	for _, o := range overrides {
		if o.SetRegistry == "" && o.SetDefaultRegistry == "" {
			continue
		}
		scopes[scopeOfPath(o.Path)] = true
	}
	return scopes
}

// scopeOfPath returns the leading subchart-name segment of a dotted
// value path (e.g. `postgresql.image` → `postgresql`, `image` → "").
// Both `global.imageRegistry` and `image` live at root scope (""), and
// both `postgresql.global.imageRegistry` and `postgresql.image` live
// under the `postgresql` subchart scope. The rule "first segment unless
// it is a Helm-special key (`global`, `image`)" yields the right answer
// for every path this function is called with — image-override paths
// always start with either a subchart name or `image`/`<componentName>`,
// never `global`. Test coverage in TestScopeOfPath documents the cases.
func scopeOfPath(path string) string {
	idx := strings.IndexByte(path, '.')
	if idx <= 0 {
		return ""
	}
	first := path[:idx]
	if first == "global" || first == "image" {
		return ""
	}
	return first
}

// buildValueOverride composes a ValueOverride that renders correctly across
// all observed upstream chart template shapes. When the upstream chart
// declares a `registry` and/or `defaultRegistry` sibling, the patched FQDN
// repository is split into `<registry>/<path>` and the registry hostname is
// emitted into those sibling fields. The wrapper leaf therefore expresses
// the FQDN compositionally — `<registry>` + `<repository>` — rather than
// concentrating the FQDN in `repository` and zeroing out the prefix (which
// produced `/ghcr.io/...:tag` leading-slash renders for every chart whose
// template was a plain `{{ registry }}/{{ repo }}` concatenation; see
// issue #308 wave 2).
//
// When the patched repo has no detectable registry host, fall back to the
// legacy "leave repo as-is" behaviour without setting the sibling — this is
// only reachable for non-FQDN patched repos, which verity does not currently
// produce, but keeping the fallback explicit avoids future surprises.
func buildValueOverride(path, patchedRepo, patchedTag string, hasRegistry, hasDefaultRegistry bool) ValueOverride {
	override := ValueOverride{
		Path:       path,
		Repository: patchedRepo,
		Tag:        patchedTag,
	}

	if !hasRegistry && !hasDefaultRegistry {
		return override
	}

	registry, repoPath, ok := splitRegistryHost(patchedRepo)
	if !ok {
		return override
	}

	override.Repository = repoPath
	if hasRegistry {
		override.SetRegistry = registry
	}
	if hasDefaultRegistry {
		override.SetDefaultRegistry = registry
	}
	return override
}

// splitRegistryHost splits a fully-qualified image reference like
// `verity.supply/grafana` into (`verity.supply`, `grafana`, true).
// A path is considered to have a registry host when its first segment
// contains either a `.` (DNS hostname), a `:` (port), or equals `localhost`
// — these are the three cases Docker's reference parser treats as a
// hostname. References without a detectable host (`library/nginx`,
// `verity-org/grafana`) return ok=false so the caller can fall back to
// the legacy behaviour.
func splitRegistryHost(repo string) (registry, path string, ok bool) {
	idx := strings.IndexByte(repo, '/')
	if idx <= 0 {
		return "", repo, false
	}
	first := repo[:idx]
	if !isRegistryHost(first) {
		return "", repo, false
	}
	return first, repo[idx+1:], true
}

// isRegistryHost mirrors the Docker reference-parser convention: the first
// path segment is a registry host when it contains a `.`, contains a `:`
// (port), or equals `localhost`. The bare-`localhost` case (no port) is the
// one easily missed by the contains-`.`-or-`:` heuristic alone.
func isRegistryHost(segment string) bool {
	if segment == "localhost" {
		return true
	}
	return strings.ContainsAny(segment, ".:")
}

func collectValuePairs(valuesYAML []byte, prefix string) ([]repoTagPair, error) {
	values := make(map[string]any)
	if len(valuesYAML) > 0 {
		if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
			return nil, fmt.Errorf("unmarshal values YAML: %w", err)
		}
	}

	pairs := make([]repoTagPair, 0)
	walkValues(prefix, values, &pairs)
	return pairs, nil
}

// collectGlobalRegistryPaths returns the dotted paths of every
// global-registry sibling in `valuesYAML`. Two patterns are recognised:
//
//   - Bitnami: `global.imageRegistry` (scalar string under the `global` map).
//   - Grafana / tempo-distributed / argo-rollouts: `global.image.registry`
//     (scalar string under the `global.image` map).
//
// Both patterns may co-exist in a single chart. The prefix is prepended
// to make subchart-scoped paths (`postgresql.global.imageRegistry` etc.).
//
// The function deliberately does NOT walk arbitrary other `global.<X>`
// scalars — chart-gen should not attempt to neutralise unrecognised
// global-scope fields. New patterns will surface as new test cases here
// when a chart-integration nightly catches them.
func collectGlobalRegistryPaths(valuesYAML []byte, prefix string) ([]string, error) {
	values := make(map[string]any)
	if len(valuesYAML) > 0 {
		if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
			return nil, fmt.Errorf("unmarshal values YAML: %w", err)
		}
	}

	globalNode, ok := values["global"].(map[string]any)
	if !ok {
		return nil, nil
	}

	scopePrefix := ""
	if prefix != "" {
		scopePrefix = prefix + "."
	}

	paths := make([]string, 0, 2)
	if _, ok := globalNode["imageRegistry"].(string); ok {
		paths = append(paths, scopePrefix+"global.imageRegistry")
	}
	if imgNode, ok := globalNode["image"].(map[string]any); ok {
		if _, ok := imgNode["registry"].(string); ok {
			paths = append(paths, scopePrefix+"global.image.registry")
		}
	}
	return paths, nil
}

func GetChartValues(chart config.ChartSpec) ([]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	args := []string{"show", "values"}
	if strings.HasPrefix(chart.Repository, "oci://") {
		args = append(args, chart.Repository+"/"+chart.Name)
	} else {
		args = append(args, chart.Name, "--repo", chart.Repository)
	}
	args = append(args, "--version", chart.Version)

	out, err := runCommand(context.Background(), 5*time.Minute, "helm", args...)
	if err != nil {
		return nil, fmt.Errorf("get chart values for %s: %w", chart.Name, err)
	}

	return []byte(out), nil
}

// GetSubchartValues fetches the parent chart dependencies and returns each
// subchart values.yaml keyed by subchart name.
func GetSubchartValues(chart config.ChartSpec) (map[string][]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "verity-chartgen-subcharts-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeSubchartDependencyChart(tmpDir, chart); err != nil {
		return nil, err
	}

	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "dependency", "build", tmpDir); err != nil {
		return nil, fmt.Errorf("helm dependency build for %s: %w", chart.Name, err)
	}

	subchartValues := make(map[string][]byte)
	chartsDir := filepath.Join(tmpDir, "charts")
	parentExtractDir := filepath.Join(tmpDir, "parent-chart")
	if err := collectNestedSubchartValues(chartsDir, parentExtractDir, subchartValues); err != nil {
		return nil, err
	}

	return subchartValues, nil
}

func writeSubchartDependencyChart(tmpDir string, chart config.ChartSpec) error {
	chartYAML, err := yaml.Marshal(map[string]any{
		"apiVersion": "v2",
		"name":       "verity-subchart-values",
		"version":    "0.0.0",
		"dependencies": []map[string]string{{
			"name":       chart.Name,
			"version":    chart.Version,
			"repository": chart.Repository,
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal temp Chart.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "Chart.yaml"), chartYAML, 0o644); err != nil {
		return fmt.Errorf("write temp Chart.yaml: %w", err)
	}
	return nil
}

func collectNestedSubchartValues(chartsDir, parentExtractDir string, subchartValues map[string][]byte) error {
	archives, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		return err
	}
	for _, archive := range archives {
		chartName, err := extractTarball(archive, parentExtractDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable parent chart archive %s: %v\n", filepath.Base(archive), err)
			continue
		}
		if err := collectParentSubcharts(filepath.Join(parentExtractDir, chartName), subchartValues); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read charts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := collectParentSubcharts(filepath.Join(chartsDir, entry.Name()), subchartValues); err != nil {
			return err
		}
	}

	return nil
}

func collectParentSubcharts(parentChartDir string, subchartValues map[string][]byte) error {
	if parentChartDir == "" {
		return nil
	}
	return collectSubchartValues(filepath.Join(parentChartDir, "charts"), subchartValues)
}

func subchartKeyFromArchive(archive, version string) string {
	base := strings.TrimSuffix(filepath.Base(archive), ".tgz")
	if version != "" {
		if stripped, ok := strings.CutSuffix(base, "-"+version); ok {
			return stripped
		}
	}
	idx := strings.LastIndex(base, "-")
	if idx > 0 && idx < len(base)-1 {
		first := base[idx+1]
		if first >= '0' && first <= '9' {
			return base[:idx]
		}
	}
	return base
}

func enumerateSubchartArchives(chartsDir string) ([]string, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read charts directory: %w", err)
	}

	archives := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".tgz" {
			continue
		}
		archives = append(archives, filepath.Join(chartsDir, entry.Name()))
	}
	sort.Strings(archives)
	return archives, nil
}

func collectSubchartValues(chartsDir string, subchartValues map[string][]byte) error {
	archives, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		return err
	}
	for _, archive := range archives {
		valuesYAML, _, version, err := readChartArchive(archive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart archive %s: %v\n", filepath.Base(archive), err)
			continue
		}
		if len(valuesYAML) == 0 {
			continue
		}
		subchartValues[subchartKeyFromArchive(archive, version)] = valuesYAML
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read charts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		valuesPath := filepath.Join(chartsDir, entry.Name(), "values.yaml")
		valuesYAML, err := os.ReadFile(valuesPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart values %s: %v\n", valuesPath, err)
			continue
		}
		subchartValues[entry.Name()] = valuesYAML
	}

	return nil
}

var (
	ErrUnsafeTarballEntry = errors.New("unsafe tarball entry path")
	ErrEmptyTarball       = errors.New("tarball contains no chart entries")
)

func safeExtractPath(destDir, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeTarballEntry, name)
	}
	target := filepath.Join(destDir, cleaned)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	return target, nil
}

func extractTarball(tgzPath, destDir string) (string, error) {
	return visitTarballEntries(tgzPath, func(header *tar.Header, name string, tarReader io.Reader) error {
		switch header.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			return nil
		}
		targetPath, err := safeExtractPath(destDir, name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(destDir, targetPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			return extractTarballDirEntry(destDir, targetPath, name)
		case tar.TypeReg:
			return extractTarballRegularEntry(tgzPath, destDir, targetPath, name, header, tarReader)
		}
		return nil
	})
}

func extractTarballDirEntry(destDir, targetPath, name string) error {
	rel, err := filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	return os.MkdirAll(targetPath, 0o755)
}

func extractTarballRegularEntry(tgzPath, destDir, targetPath, name string, header *tar.Header, tarReader io.Reader) error {
	rel, err := filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", targetPath, err)
	}
	fileMode := os.FileMode(header.Mode) & 0o777
	if fileMode == 0 {
		fileMode = 0o644
	}
	rel, err = filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("open extracted file %s: %w", targetPath, err)
	}
	_, copyErr := io.Copy(out, tarReader)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy file %s from %s: %w", name, filepath.Base(tgzPath), copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close extracted file %s: %w", targetPath, closeErr)
	}
	return nil
}

// extractValuesFromTarball reads a Helm chart tarball and returns the chart
// name together with the raw values.yaml bytes when present.
func extractValuesFromTarball(tgzPath string) (valuesYAML []byte, chartName string, err error) {
	valuesYAML, chartName, _, err = readChartArchive(tgzPath)
	return valuesYAML, chartName, err
}

func readChartArchive(tgzPath string) (valuesYAML []byte, chartName, version string, err error) {
	var chartYAML []byte
	chartName, err = visitTarballEntries(tgzPath, func(_ *tar.Header, name string, tarReader io.Reader) error {
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			return nil
		}
		switch parts[1] {
		case "values.yaml":
			data, readErr := io.ReadAll(tarReader)
			if readErr != nil {
				return fmt.Errorf("read values.yaml from %s: %w", filepath.Base(tgzPath), readErr)
			}
			valuesYAML = data
		case "Chart.yaml":
			data, readErr := io.ReadAll(tarReader)
			if readErr != nil {
				return fmt.Errorf("read Chart.yaml from %s: %w", filepath.Base(tgzPath), readErr)
			}
			chartYAML = data
		}
		return nil
	})
	if err != nil {
		return nil, "", "", err
	}
	if len(chartYAML) > 0 {
		var doc struct {
			Version string `yaml:"version"`
		}
		if unmarshalErr := yaml.Unmarshal(chartYAML, &doc); unmarshalErr == nil {
			version = doc.Version
		}
	}
	return valuesYAML, chartName, version, nil
}

func visitTarballEntries(tgzPath string, visit func(header *tar.Header, name string, tarReader io.Reader) error) (string, error) {
	file, err := os.Open(tgzPath)
	if err != nil {
		return "", fmt.Errorf("open tarball %s: %w", filepath.Base(tgzPath), err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("open gzip stream for %s: %w", filepath.Base(tgzPath), err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	chartName := ""
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			if chartName == "" {
				return "", fmt.Errorf("%w: %s", ErrEmptyTarball, filepath.Base(tgzPath))
			}
			return chartName, nil
		}
		if err != nil {
			return "", fmt.Errorf("read tarball %s: %w", filepath.Base(tgzPath), err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if name == "" {
			continue
		}

		if !filepath.IsLocal(filepath.FromSlash(name)) {
			return "", fmt.Errorf("%w: %q", ErrUnsafeTarballEntry, name)
		}

		parts := strings.Split(name, "/")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if chartName == "" {
			chartName = parts[0]
		}

		if err := visit(header, name, tarReader); err != nil {
			return "", err
		}
	}
}

func walkValues(prefix string, node map[string]any, pairs *[]repoTagPair) {
	for key, val := range node {
		child, ok := val.(map[string]any)
		if !ok {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		repo, hasRepo := child["repository"].(string)
		// `hasRegistry` / `hasDefaultRegistry` mean "the upstream chart
		// DECLARES this sibling as a string field" — we treat both
		// `registry: docker.io` and `registry: ""` as a declaration.
		// The empty-string declaration is the trickier case: if the chart
		// templates `{{ .Values.image.registry }}/{{ repository }}` and we
		// leave the wrapper leaf alone, the rendered ref becomes
		// `"" + "/" + verity.supply/...` — the same leading-slash bug this PR
		// is fixing. We must still compose the registry into the sibling
		// so the rendered ref is `verity.supply/<repo>`. The
		// previous shape (`registry != ""`) silently skipped these
		// charts; #312 review caught the gap.
		registry, hasRegistry := child["registry"].(string)
		// defaultRegistry is the kyverno 3.7.x convention: each component's
		// image map holds a hard-coded `defaultRegistry: reg.kyverno.io`
		// that the chart concatenates to the override repository when
		// `registry` is unset. Without an explicit signal, our wrapper would
		// inherit that prefix and produce broken refs like
		// `reg.kyverno.io/verity.supply/kyverno:1.17` (issue #254).
		// Treat it as a parallel registry-sibling: detect it here, plumb
		// it through SetDefaultRegistry, and have buildValuesTree write
		// `defaultRegistry: <verity.supply>` into the override leaf.
		defaultRegistry, hasDefaultRegistry := child["defaultRegistry"].(string)

		switch {
		case hasRepo && repo != "":
			_, hasTag := child["tag"]
			*pairs = append(*pairs, repoTagPair{
				Path:               path,
				Repo:               repo,
				HasTag:             hasTag,
				Registry:           registry,
				HasRegistry:        hasRegistry,
				DefaultRegistry:    defaultRegistry,
				HasDefaultRegistry: hasDefaultRegistry,
			})
		case hasRegistry:
			*pairs = append(*pairs, repoTagPair{
				Path:               path,
				Registry:           registry,
				HasRegistry:        true,
				DefaultRegistry:    defaultRegistry,
				HasDefaultRegistry: hasDefaultRegistry,
			})
		case hasDefaultRegistry:
			// Charts that declare `defaultRegistry` without `registry` or
			// `repository` at the same map (rare, but possible) still need
			// neutralisation when an explicit `valuePath` override targets
			// them. Emit a registry-only pair so the explicit-path branch
			// in resolveValuePathPairs can pick it up via
			// pathHasDefaultRegistry.
			*pairs = append(*pairs, repoTagPair{
				Path:               path,
				DefaultRegistry:    defaultRegistry,
				HasDefaultRegistry: true,
			})
		}

		walkValues(path, child, pairs)
	}
}

func matchesRepo(imageRepo, candidate string) bool {
	if imageRepo == candidate {
		return true
	}
	if strings.HasSuffix(imageRepo, "/"+candidate) {
		return true
	}
	if strings.HasSuffix(candidate, "/"+imageRepo) {
		return true
	}
	return false
}
