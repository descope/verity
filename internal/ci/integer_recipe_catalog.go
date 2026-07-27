package ci

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
	"github.com/verity-org/verity/internal/integer/melange"
)

var (
	integerPackageNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]*$`)
	integerVariablePattern    = regexp.MustCompile(`\$\{\{vars\.([A-Za-z0-9-]+)\}\}`)
)

type integerRecipe struct {
	Package struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
		Epoch   int    `yaml:"epoch"`
	} `yaml:"package"`
	Subpackages []struct {
		Name string `yaml:"name"`
	} `yaml:"subpackages"`
	Vars          map[string]string `yaml:"vars"`
	VarTransforms []struct {
		From    string `yaml:"from"`
		Match   string `yaml:"match"`
		Replace string `yaml:"replace"`
		To      string `yaml:"to"`
	} `yaml:"var-transforms"`
}

type integerRecipeDocument struct {
	Package     map[string]any   `yaml:"package"`
	Subpackages []map[string]any `yaml:"subpackages"`
}

type integerRecipeDeclaration struct {
	path        string
	fingerprint [sha256.Size]byte
}

func integerRecipePackages(
	repoRoot, imagesDir string,
	image *intdiscovery.DiscoveredImage,
) (names []string, declarations map[string]integerRecipeDeclaration, err error) {
	definitionPath := image.DefinitionFile
	if definitionPath == "" {
		definitionPath = filepath.Join(imagesDir, filepath.FromSlash(image.Name)+".yaml")
	}
	definition, err := intconfig.LoadImage(definitionPath)
	if err != nil {
		return nil, nil, fmt.Errorf("load image definition: %w", err)
	}
	configured := definition.MelangeFor(image.Version, image.Type)
	if configured == nil {
		return []string{}, map[string]integerRecipeDeclaration{}, nil
	}
	spec, err := melange.ResolveConfigSpec(configured, image.Version)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve Melange spec: %w", err)
	}
	paths := melange.DefaultPaths(repoRoot)
	recipePaths, err := integerRecipePaths(&paths, spec)
	if err != nil {
		return nil, nil, err
	}
	slices.Sort(recipePaths)
	declarations = map[string]integerRecipeDeclaration{}
	for _, recipePath := range recipePaths {
		recipeDeclarations, err := loadIntegerRecipeDeclarations(recipePath)
		if err != nil {
			return nil, nil, err
		}
		for name, declaration := range recipeDeclarations {
			if err := coalesceIntegerRecipeDeclaration(declarations, name, declaration); err != nil {
				return nil, nil, err
			}
		}
	}
	names = make([]string, 0, len(declarations))
	for name := range declarations {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, declarations, nil
}

func integerRecipePaths(paths *melange.Paths, spec melange.Spec) ([]string, error) {
	if spec.Upstream != "" {
		lock, err := loadIntegerBuildLock(paths.LockFile)
		if err != nil {
			return nil, err
		}
		entry, exists := lock.Packages[spec.Upstream]
		if !exists {
			return nil, fmt.Errorf("%w: missing locked recipe %s", ErrIntegerBatchPlan, spec.Upstream)
		}
		return []string{filepath.Join(paths.LockedDir, filepath.FromSlash(entry.File))}, nil
	}
	recipePaths := make([]string, 0, len(spec.Bespoke))
	for _, file := range spec.Bespoke {
		recipePaths = append(recipePaths, filepath.Join(paths.BespokeDir, filepath.FromSlash(file)))
	}
	return recipePaths, nil
}

func loadIntegerRecipeDeclarations(path string) (map[string]integerRecipeDeclaration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recipe %q: %w", path, err)
	}
	var recipe integerRecipe
	if err := yaml.Unmarshal(data, &recipe); err != nil {
		return nil, fmt.Errorf("parse recipe %q: %w", path, err)
	}
	var document integerRecipeDocument
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse recipe declarations %q: %w", path, err)
	}
	if !integerPackageNamePattern.MatchString(recipe.Package.Name) {
		return nil, fmt.Errorf("%w: invalid package name %q in %s", ErrIntegerBatchPlan, recipe.Package.Name, path)
	}
	variables, err := integerRecipeVariables(&recipe)
	if err != nil {
		return nil, fmt.Errorf("resolve recipe variables in %s: %w", path, err)
	}
	declarations := make(map[string]integerRecipeDeclaration, 1+len(recipe.Subpackages))
	declaration, err := newIntegerRecipeDeclaration(
		path, recipe.Package.Name, recipe.Package.Version, recipe.Package.Epoch, document.Package,
	)
	if err != nil {
		return nil, err
	}
	declarations[recipe.Package.Name] = declaration
	for index, subpackage := range recipe.Subpackages {
		name := strings.ReplaceAll(subpackage.Name, "${{package.name}}", recipe.Package.Name)
		name = integerVariablePattern.ReplaceAllStringFunc(name, func(match string) string {
			parts := integerVariablePattern.FindStringSubmatch(match)
			return variables[parts[1]]
		})
		if strings.Contains(name, "${{") || !integerPackageNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%w: unresolved subpackage name %q in %s", ErrIntegerBatchPlan, subpackage.Name, path)
		}
		if index >= len(document.Subpackages) {
			return nil, fmt.Errorf("%w: missing subpackage declaration %q in %s", ErrIntegerBatchPlan, name, path)
		}
		declaration, err := newIntegerRecipeDeclaration(
			path, name, recipe.Package.Version, recipe.Package.Epoch, document.Subpackages[index],
		)
		if err != nil {
			return nil, err
		}
		if err := coalesceIntegerRecipeDeclaration(declarations, name, declaration); err != nil {
			return nil, err
		}
	}
	return declarations, nil
}

func newIntegerRecipeDeclaration(
	path, name, version string,
	epoch int,
	fields map[string]any,
) (integerRecipeDeclaration, error) {
	semantic := map[string]any{"name": name, "version": version, "epoch": epoch}
	if dependencies, exists := fields["dependencies"]; exists {
		semantic["dependencies"] = dependencies
	}
	data, err := json.Marshal(semantic)
	if err != nil {
		return integerRecipeDeclaration{}, fmt.Errorf("marshal package declaration in %s: %w", path, err)
	}
	return integerRecipeDeclaration{path: path, fingerprint: sha256.Sum256(data)}, nil
}

func coalesceIntegerRecipeDeclaration(
	declarations map[string]integerRecipeDeclaration,
	name string,
	candidate integerRecipeDeclaration,
) error {
	previous, exists := declarations[name]
	if !exists {
		declarations[name] = candidate
		return nil
	}
	if previous.fingerprint != candidate.fingerprint {
		paths := []string{previous.path, candidate.path}
		slices.Sort(paths)
		return fmt.Errorf("%w: package %s is declared by %s and %s", ErrIntegerPackageDuplicate, name, paths[0], paths[1])
	}
	if candidate.path < previous.path {
		declarations[name] = candidate
	}
	return nil
}

func integerRecipeVariables(recipe *integerRecipe) (map[string]string, error) {
	variables := make(map[string]string, len(recipe.Vars)+len(recipe.VarTransforms))
	maps.Copy(variables, recipe.Vars)
	for _, transform := range recipe.VarTransforms {
		from := strings.ReplaceAll(transform.From, "${{package.version}}", recipe.Package.Version)
		matcher, err := regexp.Compile(transform.Match)
		if err != nil {
			return nil, fmt.Errorf("compile transform %q: %w", transform.To, err)
		}
		variables[transform.To] = matcher.ReplaceAllString(from, transform.Replace)
	}
	return variables, nil
}
