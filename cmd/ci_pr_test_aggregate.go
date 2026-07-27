package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/urfave/cli/v3"
)

type prAggregateInput struct {
	ChangesResult, DiscoverResult, ValidateResult, DetectIntegerResult string
	IntegerSmokeResult, IntegerBuildResult                             string
	DetectCopaResult, CopaChangedResult, CopaRegressionResult          string
	Integer, Copa, IntegerHasChanges                                   bool
	ExpectedIntegerMatrix, ExpectedIntegerSmokeMatrix                  string
	SecurityDir                                                        string
}

func newCIPrAggregateCommand() *cli.Command {
	return &cli.Command{
		Name:  "aggregate",
		Usage: "Require exact PR job results and native Integer security evidence",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "changes-result", Required: true},
			&cli.StringFlag{Name: integerCommandName, Required: true}, &cli.StringFlag{Name: "copa", Required: true},
			&cli.StringFlag{Name: "discover-result", Required: true}, &cli.StringFlag{Name: "validate-result", Required: true},
			&cli.StringFlag{Name: "detect-integer-result", Required: true}, &cli.StringFlag{Name: "integer-has-changes", Required: true},
			&cli.StringFlag{Name: "integer-smoke-result", Required: true}, &cli.StringFlag{Name: "integer-build-result", Required: true},
			&cli.StringFlag{Name: "expected-integer-matrix", Required: true}, &cli.StringFlag{Name: "expected-integer-smoke-matrix", Required: true},
			&cli.StringFlag{Name: "detect-copa-result", Required: true}, &cli.StringFlag{Name: "copa-changed-result", Required: true},
			&cli.StringFlag{Name: "copa-regression-result", Required: true},
			&cli.StringFlag{Name: "security-dir", Value: "integer-security-results"},
		},
		Action: runCIPrAggregate,
	}
}

func runCIPrAggregate(_ context.Context, command *cli.Command) error {
	integer, err := parsePRBool(integerCommandName, command.String(integerCommandName))
	if err != nil {
		return err
	}
	copa, err := parsePRBool("copa", command.String("copa"))
	if err != nil {
		return err
	}
	integerChanges, err := parsePRBool("integer-has-changes", command.String("integer-has-changes"))
	if err != nil {
		return err
	}
	input := prAggregateInput{
		ChangesResult: command.String("changes-result"), Integer: integer, Copa: copa,
		DiscoverResult: command.String("discover-result"), ValidateResult: command.String("validate-result"),
		DetectIntegerResult: command.String("detect-integer-result"), IntegerHasChanges: integerChanges,
		IntegerSmokeResult: command.String("integer-smoke-result"), IntegerBuildResult: command.String("integer-build-result"),
		ExpectedIntegerMatrix:      command.String("expected-integer-matrix"),
		ExpectedIntegerSmokeMatrix: command.String("expected-integer-smoke-matrix"),
		DetectCopaResult:           command.String("detect-copa-result"), CopaChangedResult: command.String("copa-changed-result"),
		CopaRegressionResult: command.String("copa-regression-result"), SecurityDir: command.String("security-dir"),
	}
	writePRAggregateSummary(command.Writer, &input)
	return evaluatePRAggregate(&input)
}

func evaluatePRAggregate(input *prAggregateInput) error {
	if err := requirePRSuccess("changes", input.ChangesResult); err != nil {
		return err
	}
	if err := evaluatePRIntegerResults(input); err != nil {
		return err
	}
	return evaluatePRCopaResults(input)
}

func evaluatePRIntegerResults(input *prAggregateInput) error {
	if !input.Integer {
		return allowPRInactiveIntegerResults(input)
	}
	if err := requirePRSuccess("validate", input.ValidateResult); err != nil {
		return err
	}
	if err := requirePRSuccess("detect-changed-images", input.DetectIntegerResult); err != nil {
		return err
	}
	if input.IntegerHasChanges {
		return evaluateChangedPRInteger(input)
	}
	if err := allowPRSuccessOrSkipped("integer-smoke-test", input.IntegerSmokeResult); err != nil {
		return err
	}
	return allowPRSuccessOrSkipped("integer-build-changed", input.IntegerBuildResult)
}

func allowPRInactiveIntegerResults(input *prAggregateInput) error {
	for _, job := range []struct{ name, result string }{
		{name: "validate", result: input.ValidateResult},
		{name: "detect-changed-images", result: input.DetectIntegerResult},
		{name: "integer-smoke-test", result: input.IntegerSmokeResult},
		{name: "integer-build-changed", result: input.IntegerBuildResult},
	} {
		if err := allowPRSuccessOrSkipped(job.name, job.result); err != nil {
			return err
		}
	}
	return nil
}

func evaluatePRCopaResults(input *prAggregateInput) error {
	if input.Copa {
		if err := requirePRSuccess("discover", input.DiscoverResult); err != nil {
			return err
		}
		if err := requirePRSuccess("detect-copa-changes", input.DetectCopaResult); err != nil {
			return err
		}
		if err := allowPRSuccessOrSkipped("copa-patching-changed", input.CopaChangedResult); err != nil {
			return err
		}
		return requirePRSuccess("copa-patching-regression", input.CopaRegressionResult)
	}
	for _, job := range []struct{ name, result string }{
		{name: "discover", result: input.DiscoverResult},
		{name: "detect-copa-changes", result: input.DetectCopaResult},
		{name: "copa-patching-changed", result: input.CopaChangedResult},
		{name: "copa-patching-regression", result: input.CopaRegressionResult},
	} {
		if err := allowPRSuccessOrSkipped(job.name, job.result); err != nil {
			return err
		}
	}
	return nil
}

func parsePRBool(name, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%w: %s must be true or false", errPRCommandFailed, name)
	}
}

func requirePRSuccess(name, result string) error {
	if result != "success" {
		return fmt.Errorf("%w: %s did not succeed: %s", errPRCommandFailed, name, result)
	}
	return nil
}

func allowPRSuccessOrSkipped(name, result string) error {
	if result != "success" && result != "skipped" {
		return fmt.Errorf("%w: %s did not succeed or skip cleanly: %s", errPRCommandFailed, name, result)
	}
	return nil
}

func writePRAggregateSummary(writer io.Writer, input *prAggregateInput) {
	_, _ = fmt.Fprintf(
		writer,
		"changes: %s\ninteger: %t\ncopa: %t\ndiscover: %s\nvalidate: %s\n"+
			"detect-changed-images: %s\ninteger-has-changes: %t\ninteger-smoke-test: %s\n"+
			"integer-build-changed: %s\ndetect-copa-changes: %s\ncopa-patching-changed: %s\n"+
			"copa-patching-regression: %s\n",
		input.ChangesResult, input.Integer, input.Copa, input.DiscoverResult, input.ValidateResult,
		input.DetectIntegerResult, input.IntegerHasChanges, input.IntegerSmokeResult,
		input.IntegerBuildResult, input.DetectCopaResult, input.CopaChangedResult, input.CopaRegressionResult,
	)
}
