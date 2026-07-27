package retry

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidPolicy = errors.New("invalid retry policy")

type Sleeper interface {
	Wait(context.Context, time.Duration) error
}

type Random interface {
	Intn(int) (int, error)
}

type Policy struct {
	MaxAttempts    int
	BaseDelay      time.Duration
	Jitter         time.Duration
	AttemptTimeout time.Duration
}

func (policy Policy) validate() error {
	if policy.MaxAttempts < 1 {
		return fmt.Errorf("%w: max attempts must be positive", ErrInvalidPolicy)
	}
	if policy.BaseDelay < 0 {
		return fmt.Errorf("%w: base delay must be non-negative", ErrInvalidPolicy)
	}
	if policy.Jitter < 0 {
		return fmt.Errorf("%w: jitter must be non-negative", ErrInvalidPolicy)
	}
	if policy.AttemptTimeout < 0 {
		return fmt.Errorf("%w: attempt timeout must be non-negative", ErrInvalidPolicy)
	}
	return nil
}

type Engine struct {
	Sleeper Sleeper
	Random  Random
	Observe func(Event) error
}

type Event struct {
	Attempt int
	Delay   time.Duration
	Err     error
}

func (engine Engine) Do(ctx context.Context, policy Policy, operation func(context.Context, int) error) error {
	if err := engine.validate(policy); err != nil {
		return err
	}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		done, err := engine.executeAttempt(ctx, policy, attempt, operation)
		if done {
			return err
		}
	}
	return nil
}

func (engine Engine) validate(policy Policy) error {
	if err := policy.validate(); err != nil {
		return err
	}
	if policy.Jitter > 0 && engine.Random == nil {
		return fmt.Errorf("%w: random source is required for jitter", ErrInvalidPolicy)
	}
	if policy.MaxAttempts > 1 && (policy.BaseDelay > 0 || policy.Jitter > 0) && engine.Sleeper == nil {
		return fmt.Errorf("%w: sleeper is required for delayed retries", ErrInvalidPolicy)
	}
	return nil
}

func (engine Engine) executeAttempt(ctx context.Context, policy Policy, attempt int, operation func(context.Context, int) error) (done bool, retErr error) {
	if err := ctx.Err(); err != nil {
		return true, err
	}
	err := runAttempt(ctx, policy.AttemptTimeout, attempt, operation)
	if err == nil {
		return true, nil
	}
	var permanent *permanentError
	if errors.As(err, &permanent) || errors.Is(err, context.DeadlineExceeded) {
		return true, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return true, ctxErr
	}
	if attempt == policy.MaxAttempts {
		return true, fmt.Errorf("retry exhausted after %d attempt(s): %w", attempt, err)
	}
	if err := engine.waitBeforeRetry(ctx, policy, attempt, err); err != nil {
		return true, err
	}
	return false, nil
}

func runAttempt(ctx context.Context, timeout time.Duration, attempt int, operation func(context.Context, int) error) error {
	if timeout <= 0 {
		return operation(ctx, attempt)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return operation(attemptCtx, attempt)
}

func (engine Engine) waitBeforeRetry(ctx context.Context, policy Policy, attempt int, attemptErr error) error {
	delay := policy.BaseDelay * time.Duration(attempt)
	if policy.Jitter > 0 {
		jitter, err := engine.Random.Intn(int(policy.Jitter))
		if err != nil {
			return fmt.Errorf("generate retry jitter: %w", err)
		}
		if jitter < 0 || jitter >= int(policy.Jitter) {
			return fmt.Errorf("%w: random jitter is outside range", ErrInvalidPolicy)
		}
		delay += time.Duration(jitter)
	}
	if engine.Observe != nil {
		if err := engine.Observe(Event{Attempt: attempt, Delay: delay, Err: attemptErr}); err != nil {
			return fmt.Errorf("observe retry: %w", err)
		}
	}
	if delay == 0 {
		return ctx.Err()
	}
	if err := engine.Sleeper.Wait(ctx, delay); err != nil {
		return fmt.Errorf("wait before retry: %w", err)
	}
	return ctx.Err()
}

type permanentError struct {
	err error
}

func (err *permanentError) Error() string {
	return err.err.Error()
}

func (err *permanentError) Unwrap() error {
	return err.err
}

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}
