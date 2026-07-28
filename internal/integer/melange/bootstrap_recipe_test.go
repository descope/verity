package melange

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func Test_NativeBootstrapRecipes_useReliableOfflineSmokeCommands(t *testing.T) {
	type testCase struct {
		name      string
		filename  string
		required  []string
		forbidden []string
	}
	tests := make([]testCase, 0, 8)
	tests = append(
		tests,
		testCase{
			name:      "karpenter consumes strings output",
			filename:  "karpenter-1.11.yaml",
			required:  []string{`strings /usr/bin/controller | grep -F "${{package.version}}"`},
			forbidden: []string{`grep -F -m1 "${{package.version}}"`},
		},
		testCase{
			name:      "minio matches tab-delimited module metadata",
			filename:  "minio-operator.yaml",
			required:  []string{`grep -F "github.com/prometheus/prometheus"`, `grep -F "v0.311.3"`},
			forbidden: []string{`grep -F "github.com/prometheus/prometheus v0.311.3"`},
		},
		testCase{
			name:      "kor matches canonical MIT grant text",
			filename:  "kor.yaml",
			required:  []string{`grep -F "Permission is hereby granted, free of charge"`},
			forbidden: []string{`grep -F "MIT License"`},
		},
	)
	for _, version := range []string{"1.14", "1.15", "1.16", "1.17", "1.18"} {
		tests = append(tests, testCase{
			name:      "kyverno " + version + " uses root help",
			filename:  "kyverno-" + version + ".yaml",
			required:  []string{"kyverno --help"},
			forbidden: []string{"kyverno version --help"},
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := nativeRecipeTestScript(t, test.filename)
			for _, required := range test.required {
				require.Contains(t, script, required)
			}
			for _, forbidden := range test.forbidden {
				require.NotContains(t, script, forbidden)
			}
		})
	}
}

func nativeRecipeTestScript(t *testing.T, filename string) string {
	t.Helper()
	paths := repositoryTestPaths(t)
	data, err := os.ReadFile(filepath.Join(paths.BespokeDir, filename))
	require.NoError(t, err)
	var recipe struct {
		Test struct {
			Pipeline []struct {
				Runs string `yaml:"runs"`
			} `yaml:"pipeline"`
		} `yaml:"test"`
	}
	require.NoError(t, yaml.Unmarshal(data, &recipe))
	scripts := make([]string, 0, len(recipe.Test.Pipeline))
	for _, step := range recipe.Test.Pipeline {
		scripts = append(scripts, step.Runs)
	}
	return strings.Join(scripts, "\n")
}
