package melange

import (
	"errors"
	"fmt"
)

var (
	errCopySourceNotRegular      = errors.New("copy source is not a regular file")
	errCopyDestinationNotRegular = errors.New("copy destination is not a regular file")
)

func copyFile(root, source, dest string) error {
	relative, err := relativeToRoot(root, source)
	if err != nil {
		return fmt.Errorf("%w: %s", errCopySourceNotRegular, source)
	}
	data, err := readRegularFile(root, relative)
	if err != nil {
		if errors.Is(err, errNotRegularFile) || errors.Is(err, errPathContainsSymlink) || errors.Is(err, errNotRealDirectory) {
			return fmt.Errorf("%w: %s", errCopySourceNotRegular, source)
		}
		return fmt.Errorf("read %s: %w", source, err)
	}
	return replaceRegularFile(root, dest, data, 0o644, errCopyDestinationNotRegular)
}
