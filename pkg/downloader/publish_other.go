//go:build !darwin

package downloader

import "os"

func publishExclusive(source, destination string) error {
	return os.Link(source, destination)
}
