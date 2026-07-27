package patchimage

import (
	"context"
	"fmt"
	"strings"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

type PlatformSpanInput struct {
	OtelPath      string
	ReportPath    string
	ImageName     string
	Platform      string
	SourceTag     string
	CVEBefore     string
	CopaExit      string
	CopaDuration  string
	StagingDigest string
}

func EmitPlatformSpan(ctx context.Context, runner retry.Runner, input *PlatformSpanInput) error {
	if runner == nil {
		runner = retry.ExecRunner{}
	}
	packageFingerprint := "unknown"
	if summary, err := ReadTrivyReport(input.ReportPath); err == nil {
		packageFingerprint = summary.PackageFingerprint()
	}
	attributes := strings.Join([]string{
		"image=" + input.ImageName,
		"platform=" + input.Platform,
		"source_tag=" + input.SourceTag,
		"cve_before=" + input.CVEBefore,
		"copa_exit=" + input.CopaExit,
		"copa_duration_seconds=" + input.CopaDuration,
		"staging_digest=" + input.StagingDigest,
		"package_list_sha256=" + packageFingerprint,
		"deployment.environment=verity-prod",
	}, ",")
	if _, err := runner.Run(ctx, &retry.Command{Name: input.OtelPath, Args: []string{
		"span", "--name", "patch-image.matrix", "--service", "verity-ci", "--kind", "internal", "--attrs", attributes,
	}}); err != nil {
		return fmt.Errorf("emit platform telemetry span: %w", err)
	}
	return nil
}
