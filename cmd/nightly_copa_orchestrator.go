package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/urfave/cli/v3"
)

type copaEvent string

const (
	copaEventPush             copaEvent = "push"
	copaEventSchedule         copaEvent = "schedule"
	copaEventWorkflowCall     copaEvent = "workflow_call"
	copaEventWorkflowDispatch copaEvent = "workflow_dispatch"
)

var (
	errInvalidCopaEvent      = errors.New("invalid COPA orchestrator event")
	errInvalidCopaImage      = errors.New("invalid COPA image name")
	errCopaMirrorUnavailable = errors.New("no BuildKit image available")
	copaImagePattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9./_-]*$`)
	copaMirrorCopy           = func(ctx context.Context, source, target string) error {
		return crane.Copy(source, target, crane.WithContext(ctx))
	}
	copaMirrorDigest = func(ctx context.Context, target string) (string, error) {
		return crane.Digest(target, crane.WithContext(ctx))
	}
)

type copaOrchestratorPlanRequest struct {
	event        copaEvent
	preflight    bool
	changeMode   copaChangeMode
	changeFilter string
	image        string
}

type copaOrchestratorPlan struct {
	only  string
	force bool
}

var nightlyCopaOrchestratorCmd = &cli.Command{
	Name:  "copa-orchestrator",
	Usage: "Run typed COPA workflow orchestration operations",
	Commands: []*cli.Command{
		nightlyCopaChangesCmd,
		nightlyCopaOrchestratorPlanCmd,
		nightlyCopaMirrorCmd,
	},
}

var nightlyCopaOrchestratorPlanCmd = &cli.Command{
	Name:  "plan",
	Usage: "Resolve workflow event policy and emit the exact COPA patch matrix",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "event-name", Required: true},
		&cli.BoolFlag{Name: "preflight"},
		&cli.StringFlag{Name: "change-mode"},
		&cli.StringFlag{Name: "change-filter"},
		&cli.StringFlag{Name: "image"},
		&cli.StringFlag{Name: "family", Value: nightlyFamilyCopa},
		&cli.StringFlag{Name: "config", Value: "copa-config.yaml"},
		&cli.StringFlag{Name: "charts-file", Value: "Chart.yaml"},
		&cli.StringFlag{Name: "verity-config", Value: "verity.yaml"},
		&cli.StringFlag{Name: "images-dir", Value: "images"},
		&cli.StringFlag{Name: "target-registry"},
		&cli.StringFlag{Name: "only"},
		&cli.IntFlag{Name: "parallel", Value: 6},
		&cli.BoolFlag{Name: "force"},
		&cli.StringFlag{Name: "output"},
		&cli.StringFlag{Name: "github-output"},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		event, err := parseCopaEvent(command.String("event-name"))
		if err != nil {
			return err
		}
		selection, err := planCopaOrchestrator(copaOrchestratorPlanRequest{
			event:        event,
			preflight:    command.Bool("preflight"),
			changeMode:   copaChangeMode(command.String("change-mode")),
			changeFilter: command.String("change-filter"),
			image:        command.String("image"),
		})
		if err != nil {
			return err
		}
		for _, flag := range []struct{ name, value string }{
			{name: "family", value: nightlyFamilyCopa},
			{name: "only", value: selection.only},
			{name: "force", value: strconv.FormatBool(selection.force)},
		} {
			if err := command.Set(flag.name, flag.value); err != nil {
				return fmt.Errorf("setting %s: %w", flag.name, err)
			}
		}
		return nightlyPlanCmd.Action(ctx, command)
	},
}

var nightlyCopaMirrorCmd = &cli.Command{
	Name:  "mirror",
	Usage: "Mirror BuildKit and accept an already-present exact target as fallback",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "source", Required: true},
		&cli.StringFlag{Name: "target", Required: true},
	},
	Action: func(ctx context.Context, command *cli.Command) error {
		if err := mirrorCopaImage(ctx, command.String("source"), command.String("target")); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "BuildKit mirror is available")
		return nil
	},
}

func parseCopaEvent(value string) (copaEvent, error) {
	event := copaEvent(value)
	switch event {
	case copaEventPush, copaEventSchedule, copaEventWorkflowCall, copaEventWorkflowDispatch:
		return event, nil
	default:
		return "", fmt.Errorf("%w: %q", errInvalidCopaEvent, value)
	}
}

func planCopaOrchestrator(request copaOrchestratorPlanRequest) (copaOrchestratorPlan, error) {
	if request.image != "" && !copaImagePattern.MatchString(request.image) {
		return copaOrchestratorPlan{}, fmt.Errorf("%w: %q", errInvalidCopaImage, request.image)
	}
	selection := copaOrchestratorPlan{only: request.image}
	switch request.event {
	case copaEventWorkflowCall, copaEventWorkflowDispatch:
		selection.force = !request.preflight
	case copaEventPush:
		if request.changeMode == copaChangeModeFilter && request.changeFilter != "" {
			selection.only = request.changeFilter
			selection.force = true
		}
	case copaEventSchedule:
	default:
		return copaOrchestratorPlan{}, fmt.Errorf("%w: %q", errInvalidCopaEvent, request.event)
	}
	return selection, nil
}

func mirrorCopaImage(ctx context.Context, source, target string) error {
	copyErr := copaMirrorCopy(ctx, source, target)
	if copyErr == nil {
		return nil
	}
	if _, digestErr := copaMirrorDigest(ctx, target); digestErr == nil {
		return nil
	} else {
		return errors.Join(
			errCopaMirrorUnavailable,
			fmt.Errorf("copying %s to %s: %w", source, target, copyErr),
			fmt.Errorf("resolving fallback %s: %w", target, digestErr),
		)
	}
}
