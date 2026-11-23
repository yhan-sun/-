package hgapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	SiteOrigin = "https://hongguoduanju.com"
	WebUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

type Episode struct {
	Vid          string `json:"vid"`
	VidIndex     int    `json:"vid_index"`
	Title        string `json:"title"`
	SeriesID     string `json:"series_id"`
	SeriesTitle  string `json:"series_title"`
	Cover        string `json:"cover"`
	WebAvailable bool   `json:"web_available"`
	Locked       bool   `json:"locked"`
}

type SeriesDetail struct {
	SeriesID              string    `json:"series_id"`
	SeriesTitle           string    `json:"series_title"`
	Cover                 string    `json:"cover"`
	Total                 int       `json:"total"`
	WebAccessibleEpisodes int       `json:"web_accessible_episodes"`
	Episodes              []Episode `json:"episodes"`
	Source                string    `json:"source"`
}

var (
	routerDataRegex = regexp.MustCompile(`(?:window\.)?_ROUTER_DATA\s*=\s*(\{)`)
)

func extractRouterData(html string) (map[string]interface{}, error) {
	loc := routerDataRegex.FindStringIndex(html)
	if loc == nil {
		return nil, fmt.Errorf("router data not found in HTML")
	}
	start := loc[1] - 1
	depth := 0
	quoted := false
	escaped := false

	for i := start; i < len(html); i++ {
		ch := html[i]
		if quoted {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				quoted = false
			}
		} else if ch == '"' {
			quoted = true
		} else if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				var data map[string]interface{}
				if err := json.Unmarshal([]byte(html[start:i+1]), &data); err != nil {
					return nil, fmt.Errorf("failed to unmarshal router data: %w", err)
				}
				return data, nil
			}
		}
	}
	return nil, fmt.Errorf("incomplete router data JSON")
}

// FetchSeriesEpisodes fetches full episode list for a series from hongguoduanju.com web endpoint.
func FetchSeriesEpisodes(ctx context.Context, seriesID string) (*SeriesDetail, error) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return nil, fmt.Errorf("empty series_id")
	}

	urls := []string{
		fmt.Sprintf("%s/player/%s", SiteOrigin, seriesID),
		fmt.Sprintf("%s/detail?series_id=%s", SiteOrigin, seriesID),
	}

	// Direct transport (no proxy) to avoid proxy hangs on domestic CDN
	tr := &http.Transport{
		Proxy: nil,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	var lastErr error
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", WebUA)
		req.Header.Set("Referer", SiteOrigin+"/")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		routerData, err := extractRouterData(string(body))
		if err != nil {
			lastErr = err
			continue
		}

		loaderData, _ := routerData["loaderData"].(map[string]interface{})
		if loaderData == nil {
			lastErr = fmt.Errorf("no loaderData in routerData")
			continue
		}

		var pageData map[string]interface{}
		for k, v := range loaderData {
			if strings.HasPrefix(k, "player_") || k == "detail_page" {
				if m, ok := v.(map[string]interface{}); ok {
					pageData = m
					break
				}
			}
		}
		if pageData == nil {
			lastErr = fmt.Errorf("no player or detail page data found")
			continue
		}

		detail, _ := pageData["seriesDetail"].(map[string]interface{})
		if detail == nil {
			lastErr = fmt.Errorf("no seriesDetail found")
			continue
		}

		sid, _ := detail["series_id"].(string)
		if sid == "" {
			if n, ok := detail["series_id"].(float64); ok {
				sid = fmt.Sprintf("%.0f", n)
			}
		}
		seriesTitle, _ := detail["series_name"].(string)
		cover, _ := detail["series_cover"].(string)
		accessibleCnt := 0
		if n, ok := detail["accessible_episode_cnt"].(float64); ok {
			accessibleCnt = int(n)
		}

		vidListRaw, _ := detail["vid_list"].([]interface{})
		if len(vidListRaw) == 0 {
			lastErr = fmt.Errorf("empty vid_list")
			continue
		}

		episodes := make([]Episode, len(vidListRaw))
		for i, v := range vidListRaw {
			vid := fmt.Sprintf("%v", v)
			episodes[i] = Episode{
				Vid:          vid,
				VidIndex:     i + 1,
				Title:        fmt.Sprintf("第%d集", i+1),
				SeriesID:     sid,
				SeriesTitle:  seriesTitle,
				Cover:        cover,
				WebAvailable: i < accessibleCnt,
			}
		}

		return &SeriesDetail{
			SeriesID:              sid,
			SeriesTitle:           seriesTitle,
			Cover:                 cover,
			Total:                 len(episodes),
			WebAccessibleEpisodes: accessibleCnt,
			Episodes:              episodes,
			Source:                "web",
		}, nil
	}

	return nil, fmt.Errorf("failed to fetch series episodes: %w", lastErr)
}
