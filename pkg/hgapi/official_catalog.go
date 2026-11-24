package hgapi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	officialWebClient = &http.Client{
		Transport: &http.Transport{
			Proxy:           nil,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 12 * time.Second,
	}

	catalogCacheMu sync.RWMutex
	catalogCache   = make(map[string]cachedCatalogResult)
	rankHeatCache  = make(map[string]rankHeatInfo) // seriesID -> rank & heat info
)

type rankHeatInfo struct {
	Rank  int
	Heat  string
	Score string
}

type cachedCatalogResult struct {
	result    *CatalogResult
	expiresAt time.Time
}

// Map user/UI genre slugs to official website category slugs
func normalizeOfficialGenre(genre string) (officialSlug string, isRank bool) {
	g := strings.ToLower(strings.TrimSpace(genre))
	switch g {
	case "hot", "热播":
		return "", true
	case "comeback", "counterattack", "逆袭":
		return "comeback", false
	case "urban", "都市":
		return "urban", false
	case "costume", "古装":
		return "costume", false
	case "suspense", "悬疑":
		return "suspense", false
	case "romance", "sweet", "angst", "爱情", "甜宠", "虐恋":
		return "romance", false
	case "family", "家庭":
		return "family", false
	case "cute-kids", "baby", "萌宝":
		return "cute-kids", false
	case "fantasy", "玄幻":
		return "fantasy", false
	case "period", "年代":
		return "period", false
	case "legend", "godofwar", "传奇", "战神":
		return "legend", false
	case "growth", "成长":
		return "growth", false
	case "comedy", "喜剧":
		return "comedy", false
	case "sci-fi", "科幻":
		return "sci-fi", false
	case "drama", "剧情":
		return "drama", false
	case "thriller", "惊悚":
		return "thriller", false
	case "horror", "恐怖":
		return "horror", false
	case "supernatural", "志怪":
		return "supernatural", false
	case "clan", "家族":
		return "clan", false
	case "wonder", "奇幻":
		return "wonder", false
	case "youth", "青春":
		return "youth", false
	case "disaster", "灾难":
		return "disaster", false
	case "action-adventure", "动作冒险":
		return "action-adventure", false
	case "war", "战争":
		return "war", false
	case "variety", "综艺":
		return "variety", false
	default:
		return g, false
	}
}

// FetchOfficialCatalog fetches the genuine official leaderboard and category lists from hongguoduanju.com
func FetchOfficialCatalog(ctx context.Context, category, genre string, page int, forceRefresh bool) (*CatalogResult, error) {
	if page < 1 {
		page = 1
	}
	category = strings.TrimSpace(category)
	genre = strings.TrimSpace(genre)

	cacheKey := fmt.Sprintf("%s:%s:%d", category, genre, page)
	if !forceRefresh {
		catalogCacheMu.RLock()
		if cached, ok := catalogCache[cacheKey]; ok && time.Now().Before(cached.expiresAt) {
			catalogCacheMu.RUnlock()
			return cached.result, nil
		}
		catalogCacheMu.RUnlock()
	}

	officialSlug, isRank := normalizeOfficialGenre(genre)

	// Determine if this is a rank request
	if category == "hot-drama" || isRank {
		result, err := fetchOfficialRank(ctx, category)
		if err == nil && result != nil && len(result.Items) > 0 {
			cacheCatalogResult(cacheKey, result, 5*time.Minute)
			return result, nil
		}
		if err != nil {
			log.Printf("[OfficialCatalog] fetchOfficialRank failed: %v", err)
		}
	}

	// Normal category browsing
	targetCategory := "real-drama"
	switch category {
	case "comic-drama", "hot-comic-drama":
		targetCategory = "comic-drama"
	case "ai-drama", "hot-ai-drama":
		targetCategory = "ai-drama"
	case "real-drama", "":
		targetCategory = "real-drama"
	default:
		targetCategory = category
	}

	result, err := fetchOfficialCategoryPage(ctx, targetCategory, officialSlug, page)
	if err != nil {
		return nil, err
	}

	cacheCatalogResult(cacheKey, result, 5*time.Minute)
	return result, nil
}

func cacheCatalogResult(key string, res *CatalogResult, ttl time.Duration) {
	catalogCacheMu.Lock()
	catalogCache[key] = cachedCatalogResult{
		result:    res,
		expiresAt: time.Now().Add(ttl),
	}
	catalogCacheMu.Unlock()
}

func fetchOfficialRank(ctx context.Context, category string) (*CatalogResult, error) {
	rankKey := "hot-drama"
	switch category {
	case "comic-drama", "hot-comic-drama":
		rankKey = "hot-comic-drama"
	case "ai-drama", "hot-ai-drama":
		rankKey = "hot-ai-drama"
	case "real-drama", "hot-real-drama":
		rankKey = "hot-real-drama"
	default:
		rankKey = "hot-drama"
	}

	url := fmt.Sprintf("%s/rank/%s", SiteOrigin, rankKey)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", WebUA)
	req.Header.Set("Referer", SiteOrigin+"/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := officialWebClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch rank %s error: %w", rankKey, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rank %s returned HTTP %d", rankKey, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read rank body: %w", err)
	}

	routerData, err := extractRouterData(string(body))
	if err != nil {
		return nil, fmt.Errorf("extract router data from rank: %w", err)
	}

	loaderData, _ := routerData["loaderData"].(map[string]interface{})
	if loaderData == nil {
		return nil, fmt.Errorf("no loaderData in rank router data")
	}

	var rankList []interface{}
	for k, v := range loaderData {
		if strings.HasPrefix(k, "rank_") {
			if m, ok := v.(map[string]interface{}); ok {
				if content, ok := m["content"].(map[string]interface{}); ok {
					if list, ok := content["rankList"].([]interface{}); ok {
						rankList = list
						break
					}
				}
			}
		}
	}

	if len(rankList) == 0 {
		return nil, fmt.Errorf("empty rankList in %s", rankKey)
	}

	var items []DramaCatalogItem
	catalogCacheMu.Lock()
	for _, it := range rankList {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		sid := parseString(m["seriesId"])
		if sid == "" {
			sid = parseString(m["id"])
		}
		if sid == "" {
			continue
		}

		title := parseString(m["title"])
		cover := parseString(m["cover"])
		rankNum := parseInt(m["rank"])
		heat := parseString(m["heatText"])
		if heat == "" {
			heat = parseString(m["primaryMetricText"])
		}
		score := parseString(m["scoreText"])
		epCount := parseInt(m["episodeCount"])
		desc := parseString(m["description"])
		tags := parseStringSlice(m["tags"])

		epLabel := ""
		if epCount > 0 {
			epLabel = fmt.Sprintf("全%d集", epCount)
		}

		// Store in heat cache for category page enrichment
		rankHeatCache[sid] = rankHeatInfo{
			Rank:  rankNum,
			Heat:  heat,
			Score: score,
		}

		items = append(items, DramaCatalogItem{
			SeriesID:         sid,
			SeriesTitle:      title,
			Cover:            cover,
			EpisodeCount:     epCount,
			EpisodeLabel:     epLabel,
			CompletionStatus: "completed",
			Heat:             heat,
			Score:            score,
			Tags:             tags,
			Description:      desc,
			Rank:             rankNum,
		})
	}
	catalogCacheMu.Unlock()

	return &CatalogResult{
		Items:      items,
		HasMore:    false,
		NextOffset: len(items),
	}, nil
}

func fetchOfficialCategoryPage(ctx context.Context, category, genreSlug string, page int) (*CatalogResult, error) {
	var targetURL string
	if genreSlug != "" && genreSlug != "all" {
		targetURL = fmt.Sprintf("%s/category/%s/%s?page=%d", SiteOrigin, category, genreSlug, page)
	} else {
		targetURL = fmt.Sprintf("%s/category/%s?page=%d", SiteOrigin, category, page)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", WebUA)
	req.Header.Set("Referer", SiteOrigin+"/")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := officialWebClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch category %s error: %w", targetURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("category %s returned HTTP %d", targetURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read category body: %w", err)
	}

	routerData, err := extractRouterData(string(body))
	if err != nil {
		return nil, fmt.Errorf("extract router data from category: %w", err)
	}

	loaderData, _ := routerData["loaderData"].(map[string]interface{})
	if loaderData == nil {
		return nil, fmt.Errorf("no loaderData in category router data")
	}

	var catData map[string]interface{}
	for k, v := range loaderData {
		if strings.HasPrefix(k, "category_") {
			if m, ok := v.(map[string]interface{}); ok {
				if _, ok := m["recommendList"]; ok {
					catData = m
					break
				}
			}
		}
	}

	if catData == nil {
		return nil, fmt.Errorf("no category recommendList found")
	}

	recList, _ := catData["recommendList"].([]interface{})
	pagination, _ := catData["pagination"].(map[string]interface{})

	hasMore := false
	if pagination != nil {
		pageNum := parseInt(pagination["pageNum"])
		totalPages := parseInt(pagination["totalPages"])
		hasMore = pageNum < totalPages
	} else {
		hasMore = len(recList) >= 20
	}

	// Warm rank heat cache if empty and on page 1 of real-drama
	catalogCacheMu.RLock()
	needWarmRank := len(rankHeatCache) == 0 && page == 1 && category == "real-drama" && (genreSlug == "" || genreSlug == "all")
	catalogCacheMu.RUnlock()
	if needWarmRank {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, _ = fetchOfficialRank(bgCtx, "hot-real-drama")
		}()
	}

	catalogCacheMu.RLock()
	defer catalogCacheMu.RUnlock()

	var items []DramaCatalogItem
	for i, it := range recList {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}

		sid := parseString(m["series_id"])
		if sid == "" {
			continue
		}

		title := parseString(m["series_name"])
		if title == "" {
			title = parseString(m["series_title"])
		}
		cover := parseString(m["series_cover"])
		if cover == "" {
			cover = parseString(m["cover"])
		}

		epCount := parseInt(m["episode_cnt"])
		if epCount == 0 {
			epCount = parseInt(m["episode_count"])
		}

		epLabel := parseString(m["episode_right_text"])
		if epLabel == "" && epCount > 0 {
			epLabel = fmt.Sprintf("全%d集", epCount)
		}

		desc := parseString(m["series_intro"])
		if desc == "" {
			desc = parseString(m["description"])
		}

		tags := parseStringSlice(m["tags"])

		completionStatus := "completed"
		if epInfo, ok := m["series_episode_info"].(map[string]interface{}); ok {
			if parseInt(epInfo["series_status"]) == 0 {
				completionStatus = "ongoing"
			}
		}

		rankNum := 0
		heat := ""
		score := ""

		if page == 1 && (genreSlug == "" || genreSlug == "all") && i < 20 {
			rankNum = i + 1
		}

		// Check if we have rank/heat in cache
		if info, exists := rankHeatCache[sid]; exists {
			if rankNum == 0 && info.Rank > 0 {
				rankNum = info.Rank
			}
			heat = info.Heat
			score = info.Score
		}

		items = append(items, DramaCatalogItem{
			SeriesID:         sid,
			SeriesTitle:      title,
			Cover:            cover,
			EpisodeCount:     epCount,
			EpisodeLabel:     epLabel,
			CompletionStatus: completionStatus,
			Heat:             heat,
			Score:            score,
			Tags:             tags,
			Description:      desc,
			Rank:             rankNum,
		})
	}

	return &CatalogResult{
		Items:      items,
		HasMore:    hasMore,
		NextOffset: page * len(items),
	}, nil
}

func parseString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case json.Number:
		return val.String()
	case fmt.Stringer:
		return val.String()
	default:
		if val != nil {
			return strings.TrimSpace(fmt.Sprint(val))
		}
		return ""
	}
}

func parseInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	default:
		return 0
	}
}

func parseStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	res := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok && s != "" {
			res = append(res, strings.TrimSpace(s))
		}
	}
	return res
}
