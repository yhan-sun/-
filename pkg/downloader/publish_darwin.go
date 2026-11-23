package downloader

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func publishExclusive(source, destination string) error {
	err := unix.RenamexNp(source, destination, unix.RENAME_EXCL)
	if errors.Is(err, errors.ErrUnsupported) || err == unix.EINVAL {
		// Some volumes support hard links but not an exclusive rename.
		return os.Link(source, destination)
	}
	return err
}
