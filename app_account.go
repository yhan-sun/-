package main

import "hongguo/internal/accountweb"

// OpenAccountWebLogin opens the official account surface, where the user can sign in or sign out.
func (a *App) OpenAccountWebLogin(inBrowser bool) (interface{}, error) {
	if _, err := a.desktopContext(); err != nil {
		return nil, err
	}
	target, err := accountweb.Open(inBrowser)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "target": target}, nil
}
