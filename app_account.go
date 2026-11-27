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

// OpenOfficialEpisode presents the publisher's player with its own WebKit session.
func (a *App) OpenOfficialEpisode(series, episode string) (interface{}, error) {
	if _, err := a.desktopContext(); err != nil {
		return nil, err
	}
	target, err := accountweb.OpenPlayback(series, episode)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"success": true, "target": target}, nil
}
