package buildmetadata

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentValidated_accepts_valid_linker_injection(t *testing.T) {
	// Given valid release metadata supplied through the linker seam.
	withInjectedMetadata(t, "v9-custom", testSourceSHA, testBuildKey, "")

	// When the trusted metadata is resolved.
	details := testRuntimeDetails()
	info, err := resolveCurrent(&details)

	// Then every injected value is retained and the build is trusted.
	require.NoError(t, err)
	assert.Equal(t, "v9-custom", info.Version)
	assert.Equal(t, testSourceSHA, info.SourceSHA)
	assert.Equal(t, testBuildKey, info.BuildKey)
	assert.Equal(t, BuiltStatus, info.BuildStatus)
}

func TestCurrentValidated_keeps_deliberate_dev_untrusted(t *testing.T) {
	// Given deliberate development metadata with otherwise valid identity values.
	withInjectedMetadata(t, DevelopmentVersion, testSourceSHA, testBuildKey, "")

	// When metadata is resolved.
	details := testRuntimeDetails()
	info, err := resolveCurrent(&details)

	// Then development remains explicit and is never promoted to built.
	require.NoError(t, err)
	assert.Equal(t, DevelopmentVersion, info.Version)
	assert.Equal(t, DevelopmentStatus, info.BuildStatus)
}

func TestCurrentValidated_rejects_invalid_versions_without_echoing_them(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{name: "empty", version: ""},
		{name: "unknown", version: UnknownValue},
		{name: "whitespace", version: " v9-custom "},
		{name: "control", version: "v9\nsecret-version"},
		{name: "overlong", version: strings.Repeat("v", 65)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an untrusted version with otherwise valid release identity.
			withInjectedMetadata(t, test.version, testSourceSHA, testBuildKey, "")

			// When trusted metadata is resolved.
			details := testRuntimeDetails()
			_, err := resolveCurrent(&details)

			// Then resolution fails generically and safe fallback remains development.
			require.ErrorIs(t, err, ErrInvalidMetadata)
			if test.version != "" {
				assert.NotContains(t, err.Error(), test.version)
			}
			fallback := Current()
			assert.Equal(t, DevelopmentVersion, fallback.Version)
			assert.Equal(t, DevelopmentStatus, fallback.BuildStatus)
		})
	}
}

func TestCurrentValidated_rejects_freeform_build_flags_without_leaking_them(t *testing.T) {
	// Given a secret-bearing free-form linker value.
	withInjectedMetadata(t, "v9-custom", testSourceSHA, testBuildKey, "API_TOKEN=super-secret-value")

	// When trusted metadata is resolved.
	details := testRuntimeDetails()
	_, err := resolveCurrent(&details)

	// Then the value is rejected without reaching errors or safe fallback JSON.
	require.ErrorIs(t, err, ErrInvalidMetadata)
	assert.NotContains(t, err.Error(), "super-secret-value")
	data, marshalErr := MarshalInfo(Current())
	require.NoError(t, marshalErr)
	assert.NotContains(t, string(data), "super-secret-value")
}

func TestResolveCurrent_accepts_only_build_flags_recorded_by_Go(t *testing.T) {
	// Given one allowlisted injected flag that Go recorded for the binary.
	withInjectedMetadata(t, "v9-custom", testSourceSHA, testBuildKey, "-trimpath")
	details := testRuntimeDetails()
	details.BuildFlags = []string{"-trimpath"}

	// When metadata is resolved against actual runtime details.
	info, err := resolveCurrent(&details)

	// Then the canonical flag is accepted without free-form text.
	require.NoError(t, err)
	assert.Equal(t, []string{"-trimpath"}, info.BuildFlags)
}

func TestResolveCurrent_rejects_mismatched_Go_VCS_revision(t *testing.T) {
	// Given injected source identity that conflicts with Go's embedded revision.
	withInjectedMetadata(t, "v9-custom", testSourceSHA, testBuildKey, "")
	details := testRuntimeDetails()
	details.VCSRevision = strings.Repeat("b", 40)

	// When metadata is resolved.
	_, err := resolveCurrent(&details)

	// Then trusted metadata is rejected without echoing either revision.
	require.ErrorIs(t, err, ErrInvalidMetadata)
	assert.NotContains(t, err.Error(), testSourceSHA)
	assert.NotContains(t, err.Error(), details.VCSRevision)
}

func TestRuntimeDetailsFromSettings_uses_only_typed_VCS_and_flag_values(t *testing.T) {
	tests := []struct {
		name       string
		settings   map[string]string
		wantDirty  *bool
		wantStatus VCSStatus
		wantFlags  []string
	}{
		{name: "unknown", settings: map[string]string{}, wantDirty: nil, wantStatus: UnknownVCSStatus, wantFlags: []string{"-buildmode=exe", "-trimpath"}},
		{name: "clean", settings: map[string]string{"vcs.modified": "false"}, wantDirty: new(false), wantStatus: CleanVCSStatus, wantFlags: []string{"-buildmode=exe", "-buildvcs=true", "-trimpath"}},
		{name: "dirty", settings: map[string]string{"vcs.modified": "true"}, wantDirty: new(true), wantStatus: DirtyVCSStatus, wantFlags: []string{"-buildmode=exe", "-buildvcs=true", "-trimpath"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given Go build settings for one VCS state plus hostile free-form values.
			test.settings["-trimpath"] = "true"
			test.settings["-buildmode"] = "exe"
			test.settings["-ldflags"] = "API_TOKEN=super-secret-value"
			test.settings["-tags"] = "super-secret-tag"

			// When typed runtime metadata is derived.
			details := runtimeDetailsFromSettings("linux", "amd64", test.settings)

			// Then dirty state is honest and only allowlisted canonical flags survive.
			assert.Equal(t, test.wantDirty, details.Dirty)
			assert.Equal(t, test.wantStatus, details.VCSStatus)
			assert.Equal(t, test.wantFlags, details.BuildFlags)
			assert.NotContains(t, strings.Join(details.BuildFlags, " "), "secret")
		})
	}
}

func TestMarshalInfo_rejects_unsafe_build_flags(t *testing.T) {
	// Given metadata containing a value that is not a canonical build flag.
	info := Info{BuildFlags: []string{"API_TOKEN=super-secret-value"}}

	// When JSON serialization is attempted.
	data, err := MarshalInfo(info)

	// Then serialization fails without returning leakable bytes.
	require.ErrorIs(t, err, ErrInvalidMetadata)
	assert.Nil(t, data)
	assert.NotContains(t, err.Error(), "super-secret-value")
}

func TestCurrentValidated_rejects_malformed_injected_identity_without_echoing_it(t *testing.T) {
	tests := []struct {
		name      string
		sourceSHA string
		buildKey  string
	}{
		{name: "source SHA", sourceSHA: "secret-source-token", buildKey: testBuildKey},
		{name: "build key", sourceSHA: testSourceSHA, buildKey: "secret-build-token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given malformed metadata that must never become trusted output.
			withInjectedMetadata(t, "v9-custom", test.sourceSHA, test.buildKey, "")

			// When trusted metadata is resolved.
			details := testRuntimeDetails()
			_, err := resolveCurrent(&details)

			// Then resolution fails generically without leaking the injected value.
			require.ErrorIs(t, err, ErrInvalidMetadata)
			assert.NotContains(t, err.Error(), "secret-")

			fallback := Current()
			assert.Equal(t, DevelopmentVersion, fallback.Version)
			assert.Equal(t, UnknownValue, fallback.SourceSHA)
			assert.Equal(t, UnknownValue, fallback.BuildKey)
			data, marshalErr := MarshalInfo(fallback)
			require.NoError(t, marshalErr)
			assert.NotContains(t, string(data), "secret-")
		})
	}
}

func TestMarshalInfo_is_deterministic_with_complete_metadata(t *testing.T) {
	// Given complete metadata with deliberately unsorted build flags.
	info := Info{
		Version:     "v9-custom",
		SourceSHA:   testSourceSHA,
		BuildKey:    testBuildKey,
		GoVersion:   "go1.26.5",
		GOOS:        "linux",
		GOARCH:      "amd64",
		CGOEnabled:  "0",
		BuildFlags:  []string{"-trimpath", "-buildmode=exe"},
		Dirty:       new(false),
		VCSStatus:   CleanVCSStatus,
		BuildStatus: BuiltStatus,
	}

	// When the same metadata is marshaled twice.
	first, err := MarshalInfo(info)
	require.NoError(t, err)
	second, err := MarshalInfo(info)

	// Then the bytes are identical and contain no incidental whitespace.
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, "{\"version\":\"v9-custom\",\"source_sha\":\"0123456789012345678901234567890123456789\",\"build_key\":\"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\",\"go_version\":\"go1.26.5\",\"goos\":\"linux\",\"goarch\":\"amd64\",\"cgo_enabled\":\"0\",\"build_flags\":[\"-buildmode=exe\",\"-trimpath\"],\"dirty\":false,\"vcs_status\":\"clean\",\"build_status\":\"built\"}\n", string(first))
	assert.False(t, strings.Contains(string(first), "\n\n"))
	assert.True(t, strings.HasSuffix(string(first), "\n"))
}

func withInjectedMetadata(t *testing.T, injectedVersion, injectedSourceSHA, injectedBuildKey, injectedFlags string) {
	t.Helper()
	previousVersion := version
	previousSourceSHA := sourceSHA
	previousBuildKey := buildKey
	previousBuildFlags := buildFlags
	version = injectedVersion
	sourceSHA = injectedSourceSHA
	buildKey = injectedBuildKey
	buildFlags = injectedFlags
	t.Cleanup(func() {
		version = previousVersion
		sourceSHA = previousSourceSHA
		buildKey = previousBuildKey
		buildFlags = previousBuildFlags
	})
}

func testRuntimeDetails() runtimeDetails {
	return runtimeDetails{
		GOOS:        "linux",
		GOARCH:      "amd64",
		CGOEnabled:  "0",
		VCSRevision: UnknownValue,
		VCSStatus:   UnknownVCSStatus,
	}
}
