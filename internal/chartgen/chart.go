package chartgen

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

const (
	helmPushMaxAttempts = 4
	helmPushTimeout     = 5 * time.Minute
	helmPushBaseBackoff = 5 * time.Second
	helmPushMaxBackoff  = 30 * time.Second
)

type commandRunner func(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error)

type contextSleeper func(ctx context.Context, delay time.Duration) error

var errHelmPushFailed = errors.New("helm push failed")

// WrapperChart holds the generated wrapper chart YAML content.
type WrapperChart struct {
	Name       string
	Version    string
	ChartYAML  []byte
	ValuesYAML []byte
}

// BuildWrapperChart creates a wrapper Helm chart that depends on the original
// chart and overrides image values with patched references.
func BuildWrapperChart(original config.ChartSpec, chartValues map[string]any, overrides []ValueOverride) (*WrapperChart, error) {
	if original.Name == "" {
		return nil, ErrEmptyChartName
	}
	if err := discovery.ValidateChartSpec(original); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	wrapperName := original.Name
	wrapperVersion := original.Version

	chartDoc := map[string]any{
		"apiVersion":  "v2",
		"name":        wrapperName,
		"description": fmt.Sprintf("Verity-patched wrapper for %s %s", original.Name, original.Version),
		"type":        "application",
		"version":     wrapperVersion,
		"dependencies": []map[string]string{{
			"name":       original.Name,
			"version":    original.Version,
			"repository": original.Repository,
		}},
		"annotations": map[string]string{
			"verity.supply/source-chart":      original.Name,
			"verity.supply/source-version":    original.Version,
			"verity.supply/source-repository": original.Repository,
		},
	}

	chartYAML, err := yaml.Marshal(chartDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal Chart.yaml: %w", err)
	}

	valuesTree := buildValuesTree(original.Name, chartValues, overrides)
	valuesYAML, err := yaml.Marshal(valuesTree)
	if err != nil {
		return nil, fmt.Errorf("marshal values.yaml: %w", err)
	}

	return &WrapperChart{
		Name:       wrapperName,
		Version:    wrapperVersion,
		ChartYAML:  chartYAML,
		ValuesYAML: valuesYAML,
	}, nil
}

// PackageChart writes the wrapper chart to a temp dir and runs helm package.
// Returns the path to the .tgz archive. Caller is responsible for cleanup.
func PackageChart(chart *WrapperChart) (string, error) {
	if chart == nil {
		return "", ErrNilChart
	}

	tmpDir, err := os.MkdirTemp("", "verity-wrapper-chart-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	chartPath := filepath.Join(tmpDir, "Chart.yaml")
	valuesPath := filepath.Join(tmpDir, "values.yaml")

	if err := os.WriteFile(chartPath, chart.ChartYAML, 0o644); err != nil {
		return "", fmt.Errorf("write Chart.yaml: %w", err)
	}
	if err := os.WriteFile(valuesPath, chart.ValuesYAML, 0o644); err != nil {
		return "", fmt.Errorf("write values.yaml: %w", err)
	}
	// Download the declared dependency (the original chart) into charts/.
	// helm package requires dependencies to be present.
	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "dependency", "build", tmpDir); err != nil {
		return "", fmt.Errorf("helm dependency build: %w", err)
	}

	out, err := runCommand(context.Background(), 5*time.Minute, "helm", "package", tmpDir)
	if err != nil {
		return "", fmt.Errorf("helm package: %w", err)
	}

	const prefix = "Successfully packaged chart and saved it to:"
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, prefix); found {
			path := strings.TrimSpace(rest)
			if strings.HasSuffix(path, ".tgz") {
				return path, nil
			}
		}
	}

	return "", ErrNoArchivePath
}

// PushChart pushes a packaged chart archive to an OCI registry.
func PushChart(tgzPath, registry string) error {
	return pushChartWithRetry(context.Background(), tgzPath, registry, runCommand, sleepContext, helmPushMaxAttempts)
}

func pushChartWithRetry(ctx context.Context, tgzPath, registry string, runner commandRunner, sleeper contextSleeper, maxAttempts int) error {
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	details := make([]string, 0, maxAttempts)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		_, err := runner(ctx, helmPushTimeout, "helm", "push", tgzPath, registry)
		if err == nil {
			return nil
		}

		details = append(details, fmt.Sprintf("attempt %d: %v", attempt, err))
		if attempt == maxAttempts || !isRetriableHelmPushError(err) {
			return helmPushError(tgzPath, registry, attempt, maxAttempts, details)
		}

		delay := helmPushBackoff(attempt)
		fmt.Fprintf(os.Stderr, "warning: helm push %s to %s failed on attempt %d/%d; retrying in %s: %v\n", tgzPath, registry, attempt, maxAttempts, delay, err)
		if sleepErr := sleeper(ctx, delay); sleepErr != nil {
			details = append(details, fmt.Sprintf("wait before retry after attempt %d: %v", attempt, sleepErr))
			return helmPushError(tgzPath, registry, attempt, maxAttempts, details)
		}
	}

	return helmPushError(tgzPath, registry, maxAttempts, maxAttempts, details)
}

func helmPushBackoff(failedAttempt int) time.Duration {
	delay := helmPushBaseBackoff
	for i := 1; i < failedAttempt; i++ {
		if delay >= helmPushMaxBackoff/2 {
			return helmPushMaxBackoff
		}
		delay *= 2
	}
	if delay > helmPushMaxBackoff {
		return helmPushMaxBackoff
	}
	return delay
}

func isRetriableHelmPushError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	retriableFragments := []string{
		`failed to perform "tag" on destination`,
		"timeout",
		"temporary",
		"connection reset",
		"tls handshake timeout",
		"unexpected eof",
		"server closed idle connection",
		"too many requests",
		"429",
		"503 service unavailable",
		"502 bad gateway",
		"504 gateway timeout",
	}
	for _, fragment := range retriableFragments {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}

func helmPushError(tgzPath, registry string, attempts, maxAttempts int, details []string) error {
	return fmt.Errorf("%w: %s to %s after %d/%d attempt(s): %s", errHelmPushFailed, tgzPath, registry, attempts, maxAttempts, strings.Join(details, "; "))
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// buildValuesTree converts flat dotted-path overrides to a nested map scoped
// under the chart name (Helm dependency override convention).
func buildValuesTree(chartName string, chartValues map[string]any, overrides []ValueOverride) map[string]any {
	if len(chartValues) == 0 && len(overrides) == 0 {
		return map[string]any{}
	}

	root := make(map[string]any)
	chartRoot := make(map[string]any)
	root[chartName] = chartRoot

	keys := make([]string, 0, len(chartValues))
	for key := range chartValues {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		setScalarValue(chartRoot, key, chartValues[key])
	}

	for _, override := range overrides {
		// IsScalarOverride wins over the legacy `Value != ""` shape so
		// that intentional empty-string scalars (#308 wave 3 global
		// registry neutralisation) are written through. The legacy
		// branch remains for ChartImageOverride csv/single-source
		// scalars that always carry a non-empty Value.
		if override.IsScalarOverride {
			setScalarValue(chartRoot, override.Path, override.Value)
			continue
		}
		if override.Value != "" {
			setScalarValue(chartRoot, override.Path, override.Value)
			continue
		}

		leaf := map[string]any{
			"repository": override.Repository,
			"tag":        override.Tag,
		}
		// SetRegistry / SetDefaultRegistry carry the registry hostname
		// (e.g. `ghcr.io`) extracted from the patched FQDN, so the
		// chart's template `{{ <sibling> }}/{{ repository }}` composes
		// to `ghcr.io/verity-org/<repo>:<tag>` regardless of whether
		// the template short-circuits via `default` or concatenates
		// directly. Empty siblings (`""`) would produce leading-slash
		// renders for direct-concatenation templates (see #308 wave 2).
		if override.SetRegistry != "" {
			leaf["registry"] = override.SetRegistry
		}
		if override.SetDefaultRegistry != "" {
			leaf["defaultRegistry"] = override.SetDefaultRegistry
		}

		// mergeMapValue (not setScalarValue) so any sibling fields
		// already populated under override.Path by an earlier
		// chartValues entry survive — e.g. gitea's `image.rootless:
		// false` written by buildValuesTree's chartValues pass MUST
		// remain a sibling of `image.repository` / `image.tag` /
		// `image.registry` written by the image-override pass. The
		// previous setScalarValue call replaced the entire subtree at
		// override.Path with `leaf`, silently dropping the chartValues
		// sibling and triggering the gitea `:1-rootless`
		// ImagePullBackOff + missing-rootless-flag pattern documented
		// in verity-org/verity#326.
		mergeMapValue(chartRoot, override.Path, leaf)
	}

	return root
}

func setScalarValue(root map[string]any, path string, value any) {
	parts := splitOverridePath(path)
	current := root

	for i, part := range parts {
		if part == "" {
			continue
		}

		if i == len(parts)-1 {
			current[part] = value
			return
		}

		next, ok := current[part]
		if !ok {
			nextMap := make(map[string]any)
			current[part] = nextMap
			current = nextMap
			continue
		}

		nextMap, ok := next.(map[string]any)
		if !ok {
			nextMap = make(map[string]any)
			current[part] = nextMap
		}
		current = nextMap
	}
}

// mergeMapValue walks `root` along `path` and merges `value` into the
// existing leaf map at `path`. If no map exists at the leaf, it is
// created and `value`'s entries copied in. If the leaf is a non-map
// scalar, it is replaced with `value` (same fallback shape as
// setScalarValue handles when an intermediate non-map node is found).
//
// Difference from setScalarValue: setScalarValue REPLACES the leaf at
// `path` outright. That is correct for scalar overrides (a single
// string/bool/int) but wrong for map-shaped image overrides, because
// an earlier chartValues pass may have already written sibling keys
// at the same path (e.g. gitea's `image.rootless: false` next to the
// image-override pass's `image.repository` / `image.tag`). Replacing
// the whole subtree silently drops the chartValues sibling and was
// the root cause of the gitea `:1-rootless` ImagePullBackOff
// regression tracked under verity-org/verity#326.
//
// Entries in `value` win over any same-key entries already present at
// the leaf; siblings present at the leaf but absent from `value` are
// preserved.
func mergeMapValue(root map[string]any, path string, value map[string]any) {
	parts := splitOverridePath(path)
	current := root

	for i, part := range parts {
		if part == "" {
			continue
		}

		if i == len(parts)-1 {
			existing, ok := current[part].(map[string]any)
			if !ok {
				// Either no existing entry, or a non-map scalar
				// occupies the slot. Fall back to the setScalarValue
				// shape: drop a fresh map containing `value`.
				next := make(map[string]any, len(value))
				maps.Copy(next, value)
				current[part] = next
				return
			}
			maps.Copy(existing, value)
			return
		}

		next, ok := current[part]
		if !ok {
			nextMap := make(map[string]any)
			current[part] = nextMap
			current = nextMap
			continue
		}

		nextMap, ok := next.(map[string]any)
		if !ok {
			nextMap = make(map[string]any)
			current[part] = nextMap
		}
		current = nextMap
	}
}

func splitOverridePath(path string) []string {
	if path == "" {
		return nil
	}

	parts := make([]string, 0, strings.Count(path, ".")+1)
	var current strings.Builder
	inBracket := false
	var quote byte

	flush := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
	}

	for i := range len(path) {
		ch := path[i]
		if inBracket {
			if quote != 0 {
				if ch == quote {
					quote = 0
					continue
				}
				current.WriteByte(ch)
				continue
			}

			switch ch {
			case '\'', '"':
				quote = ch
			case ']':
				inBracket = false
				flush()
			case ' ':
				continue
			default:
				current.WriteByte(ch)
			}
			continue
		}

		switch ch {
		case '.':
			flush()
		case '[':
			flush()
			inBracket = true
		default:
			current.WriteByte(ch)
		}
	}

	flush()
	return parts
}
