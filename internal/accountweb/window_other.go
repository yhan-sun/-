//go:build !darwin || !cgo

package accountweb

import "github.com/pkg/browser"

func Open(_ bool) (string, error) {
	return openSite(website, "", true)
}

func openSite(target, _ string, _ bool) (string, error) {
	if err := browser.OpenURL(target); err != nil {
		return "", err
	}
	return "browser", nil
}
