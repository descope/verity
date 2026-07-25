package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

var prTrivyVersionPattern = regexp.MustCompile(`^\d[0-9A-Za-z._+-]{0,63}$`)

func newCIPrTrivyCacheKeyCommand() *cli.Command {
	return &cli.Command{
		Name:  "trivy-cache-key",
		Usage: "Emit versioned UTC-hour Trivy database cache metadata",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "github-output", Required: true},
			&cli.StringFlag{Name: "trivy", Value: "trivy"},
		},
		Action: runCIPrTrivyCacheKey,
	}
}

func runCIPrTrivyCacheKey(ctx context.Context, command *cli.Command) error {
	result, err := requirePRCommand(ctx, &prCommandRequest{Name: command.String("trivy"), Args: []string{"--version"}})
	if err != nil {
		return fmt.Errorf("read Trivy version: %w", err)
	}
	values, err := prTrivyCacheValues(string(result.Stdout), time.Now())
	if err != nil {
		return err
	}
	return appendPRGitHubValues(command.String("github-output"), values)
}

func prTrivyCacheValues(output string, now time.Time) ([][2]string, error) {
	version := ""
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "Version:" {
			version = fields[1]
			break
		}
	}
	if !prTrivyVersionPattern.MatchString(version) {
		return nil, fmt.Errorf("%w: malformed Trivy version output", errPRCommandFailed)
	}
	return [][2]string{
		{"date", now.UTC().Format("2006-01-02-15")},
		{"version", version},
	}, nil
}
