package signerlock

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_rejectsDuplicateMemberNamesBeforeTypedDecode(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{
			name: "image",
			data: `{"image":"registry.invalid/attacker","image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","runnable":true}`,
		},
		{
			name: "digest",
			data: `{"image":"` + SignerImageRepository + `","digest":"sha256:bad","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","runnable":true}`,
		},
		{
			name: "workflow",
			data: `{"image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"github.com/attacker/workflow.yaml","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","runnable":true}`,
		},
		{
			name: "source_sha",
			data: `{"image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"not-a-sha","source_sha":"` + validSourceSHA + `","runnable":true}`,
		},
		{
			name: "bootstrap",
			data: `{"image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","bootstrap":true,"bootstrap":false,"runnable":true}`,
		},
		{
			name: "runnable",
			data: `{"image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","runnable":false,"runnable":true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When a signer lock repeats a member name before typed decoding.
			_, err := Parse([]byte(tt.data))

			// Then the entire document is rejected as malformed input.
			require.Error(t, err)
			require.ErrorIs(t, err, ErrMalformed)
			require.ErrorIs(t, err, ErrDuplicateField)
		})
	}
}

func TestParse_rejectsDuplicateUnknownMemberNames(t *testing.T) {
	// Given a valid lock with an unrecognized member repeated twice.
	data := `{"image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","unknown":false,"unknown":true,"runnable":true}`

	// When the lock is parsed.
	_, err := Parse([]byte(data))

	// Then strict parsing rejects the duplicate unknown member.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
	require.ErrorIs(t, err, ErrDuplicateField)
}

func TestParse_rejectsHostileDuplicateImageAndBootstrap(t *testing.T) {
	// Given the hostile repro that hides an attacker image and bootstrap state.
	data := []byte(`{"image":"ghcr.io/attacker/apk-repository-signer","image":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","bootstrap":true,"bootstrap":false,"runnable":true}`)

	// When the exact hostile document is parsed.
	_, err := Parse(data)

	// Then parsing returns a non-nil duplicate-member error before validation.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
	require.ErrorIs(t, err, ErrDuplicateField)
	require.Contains(t, err.Error(), `"image"`)
}

func TestParse_rejectsCaseVariantAliasesAlone(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		value string
	}{
		{name: "image", alias: "IMAGE", value: `"` + SignerImageRepository + `"`},
		{name: "digest", alias: "DIGEST", value: `"` + validDigest + `"`},
		{name: "workflow", alias: "WoRkFlOw", value: `"` + TrustedWorkflowIdentity + `"`},
		{name: "source_sha", alias: "SOURCE_SHA", value: `"` + validSourceSHA + `"`},
		{name: "bootstrap", alias: "BOOTSTRAP", value: "false"},
		{name: "runnable", alias: "RuNnAbLe", value: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a complete lock that spells one field with a case-variant alias.
			data := caseVariantLock(tt.alias, tt.value)

			// When the aliased lock is parsed.
			_, err := Parse([]byte(data))

			// Then the non-canonical field name is rejected as malformed input.
			require.Error(t, err)
			require.ErrorIs(t, err, ErrMalformed)
			require.ErrorIs(t, err, ErrInvalidFieldName)
		})
	}
}

func TestParse_rejectsCaseVariantAliasesPairedWithCanonicalFields(t *testing.T) {
	tests := []struct {
		name           string
		canonical      string
		canonicalValue string
		alias          string
		aliasValue     string
	}{
		{name: "image", canonical: "image", canonicalValue: `"quay.io/attacker/apk-repository-signer"`, alias: "IMAGE", aliasValue: `"` + SignerImageRepository + `"`},
		{name: "digest", canonical: "digest", canonicalValue: `"sha256:bad"`, alias: "DIGEST", aliasValue: `"` + validDigest + `"`},
		{name: "workflow", canonical: "workflow", canonicalValue: `"github.com/attacker/workflow.yaml"`, alias: "WoRkFlOw", aliasValue: `"` + TrustedWorkflowIdentity + `"`},
		{name: "source_sha", canonical: "source_sha", canonicalValue: `"not-a-sha"`, alias: "SOURCE_SHA", aliasValue: `"` + validSourceSHA + `"`},
		{name: "bootstrap", canonical: "bootstrap", canonicalValue: "true", alias: "BOOTSTRAP", aliasValue: "false"},
		{name: "runnable", canonical: "runnable", canonicalValue: "false", alias: "RuNnAbLe", aliasValue: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given hostile canonical input followed by a trusted case-variant alias.
			data := pairedCaseVariantLock(tt.canonical, tt.canonicalValue, tt.alias, tt.aliasValue)

			// When the duplicate logical field is parsed.
			_, err := Parse([]byte(data))

			// Then the alias cannot overwrite the canonical field or bypass validation.
			require.Error(t, err)
			require.ErrorIs(t, err, ErrMalformed)
			require.ErrorIs(t, err, ErrDuplicateField)
		})
	}
}

func TestParse_rejectsExactHostileCaseVariantImageAndBootstrap(t *testing.T) {
	// Given the exact hostile repro with attacker values under canonical names and
	// trusted values under case-variant aliases.
	data := `{"image":"quay.io/attacker/apk-repository-signer","IMAGE":"` + SignerImageRepository + `","digest":"` + validDigest + `","workflow":"` + TrustedWorkflowIdentity + `","source_sha":"` + validSourceSHA + `","bootstrap":true,"BOOTSTRAP":false,"runnable":true}`

	// When the hostile document is parsed.
	_, err := Parse([]byte(data))

	// Then the parser exits with a malformed-input error instead of accepting the
	// overwritten trusted image and non-bootstrap state.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
	require.ErrorIs(t, err, ErrDuplicateField)
}

func TestParse_rejectsTrailingJSONValues(t *testing.T) {
	// Given a valid lock followed by a second JSON value.
	data := string(marshalLock(t, validLock())) + ` {"success":true}`

	// When the lock is parsed.
	_, err := Parse([]byte(data))

	// Then the trailing value cannot be treated as a successful parse.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
}

func caseVariantLock(alias, value string) string {
	return `{"` + alias + `":` + value + `,` + lockFields(canonicalFieldForAlias(alias)) + `}`
}

func pairedCaseVariantLock(canonical, canonicalValue, alias, aliasValue string) string {
	return `{"` + canonical + `":` + canonicalValue + `,"` + alias + `":` + aliasValue + `,` + lockFields(canonical) + `}`
}

func lockFields(exclude string) string {
	values := map[string]string{
		"image":      `"` + SignerImageRepository + `"`,
		"digest":     `"` + validDigest + `"`,
		"workflow":   `"` + TrustedWorkflowIdentity + `"`,
		"source_sha": `"` + validSourceSHA + `"`,
		"bootstrap":  "false",
		"runnable":   "true",
	}
	fields := make([]string, 0, len(values)-1)
	for _, name := range []string{"image", "digest", "workflow", "source_sha", "bootstrap", "runnable"} {
		if name != exclude {
			fields = append(fields, `"`+name+`":`+values[name])
		}
	}
	return strings.Join(fields, ",")
}

func canonicalFieldForAlias(alias string) string {
	switch alias {
	case "IMAGE":
		return "image"
	case "DIGEST":
		return "digest"
	case "WoRkFlOw":
		return "workflow"
	case "SOURCE_SHA":
		return "source_sha"
	case "BOOTSTRAP":
		return "bootstrap"
	case "RuNnAbLe":
		return "runnable"
	default:
		panic("unexpected test alias")
	}
}
