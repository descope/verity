package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_Dotnet8SDKFIPS_resolves_fixed_local_packages(t *testing.T) {
	// Given: the checked-in .NET image definition and locally rebuilt packages.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "dotnet.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["sdk-fips"]

	// When: the version-scoped Melange config and local package revisions are resolved.
	spec := def.MelangeFor("8", "sdk-fips")
	require.NotNil(t, spec)
	err = pinLocalPackageVersions(&tmpl, "8", []apkindex.Package{
		{Name: "dotnet-8", Version: "8.0.129-r1"},
		{Name: "dotnet-8-sdk", Version: "8.0.129-r1"},
		{Name: "openssl-provider-fips", Version: "3.1.2-r1"},
	})

	// Then: only .NET 8 FIPS builds both local recipes and pins every requested artifact.
	require.NoError(t, err)
	require.Equal(t, intconfig.StringList{"dotnet-8.yaml", "openssl-provider-fips.yaml"}, spec.Bespoke)
	require.Equal(t, intconfig.StringList{"openssl-provider-fips.yaml"}, def.MelangeFor("9", "sdk-fips").Bespoke)
	require.Equal(t, intconfig.StringList{"openssl-provider-fips.yaml"}, def.MelangeFor("10", "sdk-fips").Bespoke)
	require.Equal(t, []string{
		"dotnet-8=8.0.129-r1@local",
		"dotnet-8-sdk=8.0.129-r1@local",
		"openssl-provider-fips=3.1.2-r1@local",
	}, tmpl.Packages)
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
