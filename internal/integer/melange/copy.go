package melange

import (
	"errors"
	"fmt"
	"path/filepath"
)

var (
	errCopySourceNotRegular      = errors.New("copy source is not a regular file")
	errCopyDestinationNotRegular = errors.New("copy destination is not a regular file")
)

func copyFile(source, dest string) error {
	data, err := readRegularFile(filepath.Dir(source), filepath.Base(source))
	if err != nil {
		if errors.Is(err, errNotRegularFile) || errors.Is(err, errPathContainsSymlink) || errors.Is(err, errNotRealDirectory) {
			return fmt.Errorf("%w: %s", errCopySourceNotRegular, source)
		}
		return fmt.Errorf("read %s: %w", source, err)
	}
	return replaceRegularFile(dest, data, 0o644, errCopyDestinationNotRegular)
}
