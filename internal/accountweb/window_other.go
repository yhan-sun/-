//go:build !darwin || !cgo

package accountweb

import "github.com/pkg/browser"

func Open(_ bool) (string, error) {
	if err := browser.OpenURL(website); err != nil {
		return "", err
	}
	return "browser", nil
}
