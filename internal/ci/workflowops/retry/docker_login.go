package retry

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidDockerLogin = errors.New("invalid docker login")

type DockerLoginOptions struct {
	Registry  string
	Username  string
	Password  string
	Attempts  int
	Timeout   time.Duration
	BaseDelay time.Duration
	Jitter    time.Duration
}

func NewDockerLoginOperation(options *DockerLoginOptions) (Operation, Policy, error) {
	if options == nil {
		return Operation{}, Policy{}, ErrInvalidDockerLogin
	}
	if options.Registry == "" {
		return Operation{}, Policy{}, fmt.Errorf("%w: registry is required", ErrInvalidDockerLogin)
	}
	if options.Username == "" {
		return Operation{}, Policy{}, fmt.Errorf("%w: username is required", ErrInvalidDockerLogin)
	}
	if options.Password == "" {
		return Operation{}, Policy{}, fmt.Errorf("%w: password is required", ErrInvalidDockerLogin)
	}
	if options.Timeout <= 0 {
		return Operation{}, Policy{}, fmt.Errorf("%w: timeout must be positive", ErrInvalidDockerLogin)
	}
	if options.BaseDelay <= 0 {
		return Operation{}, Policy{}, fmt.Errorf("%w: base delay must be positive", ErrInvalidDockerLogin)
	}

	policy := Policy{MaxAttempts: options.Attempts, BaseDelay: options.BaseDelay, Jitter: options.Jitter}
	if err := policy.validate(); err != nil {
		return Operation{}, Policy{}, fmt.Errorf("%w: %w", ErrInvalidDockerLogin, err)
	}

	return Operation{
		Label: "docker login to " + options.Registry,
		Command: Command{
			Name:    "docker",
			Args:    []string{"login", options.Registry, "--username", options.Username, "--password-stdin"},
			Stdin:   []byte(options.Password),
			Timeout: options.Timeout,
		},
	}, policy, nil
}
