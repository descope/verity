package chartgen

import (
	"errors"
	"io"
	"os"
)

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	return copyToSyncWriteCloser(out, in)
}

type syncWriteCloser interface {
	io.Writer
	Sync() error
	Close() error
}

func copyToSyncWriteCloser(out syncWriteCloser, in io.Reader) (err error) {
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
