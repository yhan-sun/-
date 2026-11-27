//go:build darwin && cgo

package accountweb

/*
#cgo CFLAGS: -fobjc-arc -fblocks
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
int hgOpenAccountSite(const char *site, const char *loginScript, int inBrowser);
*/
import "C"

import (
	_ "embed"
	"errors"
	"unsafe"
)

//go:embed show-login.js
var showLogin string

// Open returns the surface that opened. Authentication takes place on that surface.
func Open(inBrowser bool) (string, error) {
	return openSite(website, showLogin, inBrowser)
}

func openSite(target, loginScript string, inBrowser bool) (string, error) {
	url, script := C.CString(target), C.CString(loginScript)
	defer C.free(unsafe.Pointer(url))
	defer C.free(unsafe.Pointer(script))

	if inBrowser {
		if C.hgOpenAccountSite(url, script, 1) == 0 {
			return "", errors.New("无法打开默认浏览器")
		}
		return "browser", nil
	}
	if C.hgOpenAccountSite(url, script, 0) == 0 {
		return "", errors.New("无法打开官网窗口，请尝试用浏览器打开")
	}
	return "window", nil
}
