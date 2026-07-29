package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Dotnet8Variants_resolve_fixed_local_packages(t *testing.T) {
	// Given: the checked-in .NET image definition and locally rebuilt packages.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "dotnet.yaml"))
	require.NoError(t, err)
	packages := []apkindex.Package{
		{Name: "dotnet-8", Version: "8.0.129-r1"},
		{Name: "dotnet-8-sdk", Version: "8.0.129-r1"},
		{Name: "dotnet-8-runtime", Version: "8.0.129-r1"},
		{Name: "aspnet-8-runtime", Version: "8.0.129-r1"},
		{Name: "openssl-provider-fips", Version: "3.1.2-r1"},
	}
	tests := []struct {
		imageType string
		bespoke   intconfig.StringList
		want      []string
	}{
		{imageType: "default", bespoke: intconfig.StringList{"dotnet-8.yaml"}, want: []string{"dotnet-8=8.0.129-r1@local", "dotnet-8-runtime=8.0.129-r1@local"}},
		{imageType: "sdk", bespoke: intconfig.StringList{"dotnet-8.yaml"}, want: []string{"dotnet-8=8.0.129-r1@local", "dotnet-8-sdk=8.0.129-r1@local"}},
		{imageType: "aspnet", bespoke: intconfig.StringList{"dotnet-8.yaml"}, want: []string{"dotnet-8=8.0.129-r1@local", "dotnet-8-runtime=8.0.129-r1@local", "aspnet-8-runtime=8.0.129-r1@local"}},
		{imageType: "sdk-fips", bespoke: intconfig.StringList{"dotnet-8.yaml", "openssl-provider-fips.yaml"}, want: []string{"dotnet-8=8.0.129-r1@local", "dotnet-8-sdk=8.0.129-r1@local", "openssl-provider-fips=3.1.2-r1@local"}},
	}

	// When: every .NET 8 variant resolves its version-scoped recipe and local package revisions.
	for _, test := range tests {
		t.Run(test.imageType, func(t *testing.T) {
			spec := def.MelangeFor("8", test.imageType)
			require.NotNil(t, spec)
			require.Equal(t, test.bespoke, spec.Bespoke)
			tmpl := def.Types[test.imageType]
			require.NoError(t, pinLocalPackageVersions(&tmpl, "8", packages))
			require.Equal(t, test.want, tmpl.Packages)
		})
	}

	// Then: versions 9 and 10 keep public .NET packages while retaining only the FIPS provider override.
	for _, imageType := range []string{"default", "sdk", "aspnet"} {
		require.Nil(t, def.MelangeFor("9", imageType))
		require.Nil(t, def.MelangeFor("10", imageType))
	}
	require.Equal(t, intconfig.StringList{"openssl-provider-fips.yaml"}, def.MelangeFor("9", "sdk-fips").Bespoke)
	require.Equal(t, intconfig.StringList{"openssl-provider-fips.yaml"}, def.MelangeFor("10", "sdk-fips").Bespoke)
}

func Test_Dotnet8_recipe_preserves_source_build_and_subpackage_contract(t *testing.T) {
	// Given: the bespoke .NET 8 recipe selected by the FIPS SDK image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "dotnet-8.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its immutable source, source-build pipeline, and package split are inspected.

	// Then: revision r1 builds v8.0.129 from its exact tag and retains the public recipe contract.
	require.Contains(t, text, "version: \"8.0.129\"")
	require.Contains(t, text, "epoch: 1")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 761968561827fbf9fd55e1259e7e0963b1a5613a")
	require.Contains(t, text, "./build.sh --clean-while-building")
	require.Contains(t, text, "Private.SourceBuilt.Artifacts.Bootstrap.tar.gz")
	require.NotContains(t, text, "uses: test/tw/")
	require.Contains(t, text, `ldd-check --packages="${{context.name}}"`)
	require.Contains(t, text, "find /usr/share/dotnet/packs/NETStandard.Library.Ref")
	for _, packageName := range []string{
		"dotnet-${{vars.major-version}}-sdk",
		"dotnet-${{vars.major-version}}-runtime",
		"aspnet-${{vars.major-version}}-runtime",
		"netstandard-${{vars.major-version}}-targeting-pack",
		"dotnet-${{vars.major-version}}-targeting-pack",
		"aspnet-${{vars.major-version}}-targeting-pack",
	} {
		require.Contains(t, text, "name: "+packageName)
	}
}
