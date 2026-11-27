// Package accountweb opens the publisher's account and playback pages.
package accountweb

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var episodeID = regexp.MustCompile(`^[0-9]{6,30}$`)

// SeriesID accepts the IDs and public detail/player URLs used by both search fields.
func SeriesID(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	if episodeID.MatchString(reference) {
		return reference, nil
	}
	u, err := url.Parse(reference)
	if err == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") {
		if id := u.Query().Get("series_id"); episodeID.MatchString(id) {
			return id, nil
		}
		if _, rest, ok := strings.Cut(u.Path, "/player/"); ok {
			if id, _, _ := strings.Cut(rest, "/"); episodeID.MatchString(id) {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("链接中找不到剧集编号，请粘贴官方详情页、播放页链接或剧集 ID")
}

func OpenPlayback(series, episode string) (string, error) {
	if !episodeID.MatchString(series) || (episode != "" && !episodeID.MatchString(episode)) {
		return "", fmt.Errorf("缺少有效的官方剧集编号")
	}
	url := website + "detail?series_id=" + series
	if episode != "" {
		url = website + "player/" + series + "/" + episode
	}
	return openSite(url, "", false)
}

const website = "https://hongguoduanju.com/"
