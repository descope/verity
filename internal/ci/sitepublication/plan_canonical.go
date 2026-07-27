package sitepublication

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

var planCodec = canonicalJSONCodec[PublicationPlan]{
	label: "site publication plan", invalid: ErrInvalidPlan, validate: validatePlan,
}

var (
	digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	sourcePattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

func MarshalPlanCanonical(plan *PublicationPlan) ([]byte, error) {
	return planCodec.marshal(plan)
}

func ParsePlanCanonical(data []byte) (PublicationPlan, error) {
	return planCodec.parse(data)
}

func digestPlan(plan *PublicationPlan) (publication.Digest, error) {
	if plan == nil {
		return "", fmt.Errorf("%w: plan is required", ErrInvalidPlan)
	}
	content := *plan
	content.PlanDigest = ""
	data, err := json.Marshal(&content)
	if err != nil {
		return "", fmt.Errorf("digest site publication plan: %w", err)
	}
	sum := sha256.Sum256(data)
	return publication.Digest("sha256:" + hex.EncodeToString(sum[:])), nil
}

func validatePlan(plan *PublicationPlan) error {
	if plan == nil || plan.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: schema version", ErrInvalidPlan)
	}
	if !digestPattern.MatchString(string(plan.ManifestDigest)) || !digestPattern.MatchString(string(plan.SignerDigest)) {
		return fmt.Errorf("%w: manifest or signer digest", ErrInvalidPlan)
	}
	if err := validatePlanMode(plan); err != nil {
		return err
	}
	if plan.RunID == 0 || plan.RunAttempt == 0 || !sourcePattern.MatchString(string(plan.SourceSHA)) ||
		!sourcePattern.MatchString(string(plan.SignerSourceSHA)) ||
		plan.BatchID != publication.BatchID(fmt.Sprintf("%d-%d", plan.RunID, plan.RunAttempt)) {
		return fmt.Errorf("%w: producer identity", ErrInvalidPlan)
	}
	if plan.SignerReference != signerlock.SignerImageRepository+"@"+string(plan.SignerDigest) {
		return fmt.Errorf("%w: signer reference", ErrInvalidPlan)
	}
	wantDigest, err := digestPlan(plan)
	if err != nil {
		return err
	}
	if plan.PlanDigest != wantDigest {
		return fmt.Errorf("%w: plan digest", ErrInvalidPlan)
	}
	return nil
}

func validatePlanMode(plan *PublicationPlan) error {
	switch plan.Mode {
	case publication.ModeBootstrap:
		if plan.PreviousManifestDigest != "" {
			return fmt.Errorf("%w: bootstrap previous digest", ErrInvalidPlan)
		}
	case publication.ModeSnapshot, publication.ModeDelta, publication.ModeRestore:
		if !digestPattern.MatchString(string(plan.PreviousManifestDigest)) {
			return fmt.Errorf("%w: previous manifest digest", ErrInvalidPlan)
		}
	default:
		return fmt.Errorf("%w: mode %q", ErrInvalidPlan, plan.Mode)
	}
	return nil
}
