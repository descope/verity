package sitepublication

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestVerifySignerInputsBase64_accepts_canonical_bound_authorization(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	encoded, err := signerAuthorizationBase64(&plan.Authorization)
	require.NoError(t, err)

	// When
	authorization, err := VerifySignerInputsBase64(
		request.WorkspaceDir,
		encoded,
		plan.InputDigest,
		plan.PublicationPlanDigest,
		plan.ManifestDigest,
	)

	// Then
	require.NoError(t, err)
	assert.Equal(t, plan.Authorization.PublicationPlanDigest, authorization.PublicationPlanDigest)
	assert.Equal(t, plan.Authorization.ManifestDigest, authorization.ManifestDigest)
	assert.Equal(t, plan.Authorization.APKOperations, authorization.APKOperations)
	assert.ElementsMatch(t, plan.Authorization.Inputs, authorization.Inputs)
	assert.ElementsMatch(t, plan.Authorization.Packages, authorization.Packages)
	reencoded, err := signerAuthorizationBase64(&authorization)
	require.NoError(t, err)
	assert.Equal(t, encoded, reencoded)
}

func TestParseSignerAuthorizationBase64_rejects_malformed_ambiguous_or_noncanonical_input(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	canonical, err := marshalSignerAuthorizationCanonical(&plan.Authorization)
	require.NoError(t, err)
	tests := []struct {
		name  string
		value string
	}{
		{name: "invalid base64", value: "%%%"},
		{name: "unknown field", value: base64.StdEncoding.EncodeToString([]byte(`{"unknown":true}`))},
		{name: "invalid authorization", value: base64.StdEncoding.EncodeToString([]byte(`{}`))},
		{name: "trailing whitespace", value: base64.StdEncoding.EncodeToString(append(append([]byte(nil), canonical...), '\n'))},
		{name: "duplicate JSON value", value: base64.StdEncoding.EncodeToString(append(append([]byte(nil), canonical...), []byte(`{}`)...))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			authorization, parseErr := parseSignerAuthorizationBase64(test.value)

			// Then
			require.ErrorIs(t, parseErr, ErrInvalidSignerPlan)
			assert.Equal(t, SignerInputAuthorization{}, authorization)
		})
	}
}

func TestValidateSignerAuthorization_rejects_invalid_or_ambiguous_inputs(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(*SignerInputAuthorization) *SignerInputAuthorization
	}{
		{name: "nil authorization", mutate: func(*SignerInputAuthorization) *SignerInputAuthorization { return nil }},
		{name: "wrong schema", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization { value.SchemaVersion++; return value }},
		{name: "invalid plan digest", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.PublicationPlanDigest = "bad"
			return value
		}},
		{name: "invalid manifest digest", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.ManifestDigest = "bad"
			return value
		}},
		{name: "nil inputs", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization { value.Inputs = nil; return value }},
		{name: "nil packages", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization { value.Packages = nil; return value }},
		{name: "unsafe input path", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.Inputs[0].Path = "../escape"
			return value
		}},
		{name: "invalid content digest", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.Inputs[0].ContentDigest = "bad"
			return value
		}},
		{name: "invalid semantic digest", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.Inputs[0].SemanticDigest = "bad"
			return value
		}},
		{name: "duplicate input", mutate: func(value *SignerInputAuthorization) *SignerInputAuthorization {
			value.Inputs = append(value.Inputs, value.Inputs[0])
			return value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			authorization := cloneSignerAuthorization(&plan.Authorization)
			candidate := test.mutate(&authorization)

			// When
			validationErr := validateSignerAuthorization(candidate)

			// Then
			require.ErrorIs(t, validationErr, ErrInvalidSignerPlan)
		})
	}
}

func TestVerifySignerInputsBase64_rejects_binding_mismatches(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	encoded, err := signerAuthorizationBase64(&plan.Authorization)
	require.NoError(t, err)
	tests := []struct {
		name   string
		mutate func(*publication.Digest, *publication.Digest, *publication.Digest)
	}{
		{name: "input digest", mutate: func(input, _, _ *publication.Digest) { *input = digestOf("f") }},
		{name: "plan digest", mutate: func(_, planDigest, _ *publication.Digest) { *planDigest = digestOf("f") }},
		{name: "manifest digest", mutate: func(_, _, manifestDigest *publication.Digest) { *manifestDigest = digestOf("f") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			inputDigest := plan.InputDigest
			planDigest := plan.PublicationPlanDigest
			manifestDigest := plan.ManifestDigest
			test.mutate(&inputDigest, &planDigest, &manifestDigest)

			// When
			_, verifyErr := VerifySignerInputsBase64(request.WorkspaceDir, encoded, inputDigest, planDigest, manifestDigest)

			// Then
			require.ErrorIs(t, verifyErr, ErrInvalidSignerPlan)
		})
	}
}

func TestVerifySignerInputsBase64_rejects_manifest_or_package_set_changes(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *SignerRequest)
	}{
		{
			name: "manifest content changed",
			mutate: func(t *testing.T, request *SignerRequest) {
				_, manifest := validPlanAndManifest(t)
				manifest.SignerDigest = digestOf("e")
				data, err := publication.MarshalCanonical(&manifest)
				require.NoError(t, err)
				writeSignerFixtureBytes(t, request, request.ManifestPath, data)
			},
		},
		{
			name: "package set changed",
			mutate: func(t *testing.T, request *SignerRequest) {
				path := filepath.Join(request.WorkspaceDir, request.PackagesPath, "extra.apk")
				require.NoError(t, os.WriteFile(path, []byte("sentinel-extra-package"), 0o644))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			publicationPlan, _ := validPlanAndManifest(t)
			request := signerRequest(t, &publicationPlan, t.TempDir())
			plan, err := BuildSignerPlan(request)
			require.NoError(t, err)
			encoded, err := signerAuthorizationBase64(&plan.Authorization)
			require.NoError(t, err)
			test.mutate(t, request)

			// When
			_, verifyErr := VerifySignerInputsBase64(
				request.WorkspaceDir,
				encoded,
				plan.InputDigest,
				plan.PublicationPlanDigest,
				plan.ManifestDigest,
			)

			// Then
			require.ErrorIs(t, verifyErr, ErrInvalidSignerPlan)
		})
	}
}

func cloneSignerAuthorization(value *SignerInputAuthorization) SignerInputAuthorization {
	clone := *value
	clone.APKOperations = append([]publication.APKOperation(nil), value.APKOperations...)
	clone.Inputs = append([]SignerAuthorizedInput(nil), value.Inputs...)
	clone.Packages = append([]SignerAuthorizedInput(nil), value.Packages...)
	return clone
}
