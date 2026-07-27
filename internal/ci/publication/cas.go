package publication

import "fmt"

func CompareAndSwap(current, expected, next Digest) error {
	if current != "" && !digestPattern.MatchString(string(current)) {
		return invalidManifest("current manifest digest %q", current)
	}
	if expected != "" && !digestPattern.MatchString(string(expected)) {
		return invalidManifest("expected manifest digest %q", expected)
	}
	if !digestPattern.MatchString(string(next)) {
		return invalidManifest("next manifest digest %q", next)
	}
	if current != expected {
		return fmt.Errorf("%w: current %s, expected %s", ErrCASMismatch, current, expected)
	}
	if current == next {
		return fmt.Errorf("%w: digest %s", ErrReplay, next)
	}
	return nil
}
