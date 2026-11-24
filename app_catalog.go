package main

import (
	"fmt"
	"log"
	"strings"

	"hongguo/pkg/device"
	"hongguo/pkg/hgapi"
)

// SearchSeries searches for dramas matching the keyword
func (a *App) SearchSeries(keyword string, options map[string]interface{}) (interface{}, error) {
	if source, _ := options["source"].(string); source != "" && source != "hongguo" {
		return nil, fmt.Errorf("当前桌面版未接入此来源")
	}
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return map[string]interface{}{
			"success": false,
			"results": []interface{}{},
			"error":   "请输入搜索关键词",
		}, nil
	}

	a.mu.Lock()
	dev := a.dev
	a.mu.Unlock()

	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err != nil {
			return map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("设备注册失败: %v", err),
			}, nil
		}
		a.mu.Lock()
		a.dev = dev
		a.registered = true
		a.mu.Unlock()
	}

	page := 1
	if p, ok := options["page"].(float64); ok && p > 0 {
		page = int(p)
	}

	items, err := hgapi.Search(keyword, page, dev)
	if err != nil {
		log.Printf("[App] Search error: %v", err)
		return map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}, nil
	}

	results := make([]map[string]interface{}, len(items))
	for i, it := range items {
		results[i] = map[string]interface{}{
			"series_id":     it.SeriesID,
			"series_title":  it.SeriesTitle,
			"cover":         it.Cover,
			"episode_count": it.EpisodeCount,
			"author":        "",
			"intro":         it.Description,
			"tags":          []string{},
			"url":           it.URL,
		}
	}

	return map[string]interface{}{
		"success": true,
		"results": results,
		"total":   len(results),
		"page":    page,
		"channel": "app",
		"hasMore": len(results) >= 20,
	}, nil
}

type catalogGenre struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

var catalogGenres = []catalogGenre{
	{"hot", "热播"},
	{"comeback", "逆袭"},
	{"urban", "都市"},
	{"costume", "古装"},
	{"romance", "甜宠/爱情"},
	{"suspense", "悬疑"},
	{"comedy", "喜剧"},
	{"fantasy", "玄幻"},
	{"period", "年代"},
	{"cute-kids", "萌宝"},
	{"family", "家庭"},
	{"legend", "传奇"},
	{"growth", "成长"},
	{"sci-fi", "科幻"},
	{"thriller", "惊悚"},
	{"horror", "恐怖"},
	{"supernatural", "志怪"},
	{"wonder", "奇幻"},
	{"youth", "青春"},
	{"clan", "家族"},
	{"action-adventure", "动作冒险"},
	{"drama", "剧情"},
	{"disaster", "灾难"},
	{"war", "战争"},
	{"variety", "综艺"},
	{"comic", "动漫"},
}

func (a *App) BrowseCategories() (interface{}, error) {
	return []catalogGenre{
		{"real-drama", "真人剧"},
		{"comic-drama", "动态漫剧"},
		{"hot-drama", "热播总榜"},
		{"hot-real-drama", "真人热榜"},
		{"hot-comic-drama", "动漫热榜"},
	}, nil
}

func catalogKeyword(category, genre string) string {
	for _, option := range catalogGenres {
		if option.Slug == genre {
			return option.Label
		}
	}
	if genre != "" && genre != "all" {
		return genre
	}
	switch category {
	case "comic-drama", "hot-comic-drama":
		return "动漫"
	case "hot-drama":
		return "热播"
	default:
		return "短剧"
	}
}

func catalogPage(items []hgapi.DramaCatalogItem, hasMore bool, category string) map[string]interface{} {
	if items == nil {
		items = []hgapi.DramaCatalogItem{}
	}
	// Neither provider reports a total. The pager uses hasMore in that case.
	return map[string]interface{}{
		"success": true, "results": items, "genres": catalogGenres, "hasMore": hasMore,
		"sourceUrl": hgapi.SiteOrigin + "/category/" + category,
	}
}

func (a *App) BrowseList(options map[string]interface{}) (interface{}, error) {
	if source, _ := options["source"].(string); source != "" && source != "hongguo" {
		return nil, fmt.Errorf("当前桌面版未接入此来源")
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return nil, fmt.Errorf("桌面服务尚未就绪，请重试")
	}

	page := 1
	if value, ok := options["page"].(float64); ok && value >= 1 {
		page = int(value)
	}
	category, _ := options["category"].(string)
	genre, _ := options["genre"].(string)
	forceRefresh, _ := options["forceRefresh"].(bool)

	// 1. Try official high-quality ranking & category catalog first (hongguoduanju.com)
	offResult, offErr := hgapi.FetchOfficialCatalog(ctx, category, genre, page, forceRefresh)
	if offErr == nil && offResult != nil && len(offResult.Items) > 0 {
		return catalogPage(offResult.Items, offResult.HasMore, category), nil
	}
	if offErr != nil {
		log.Printf("[App] Official catalog fetch failed (%v), falling back to Landpage/Search", offErr)
	}

	// 2. Fallback to App Landpage / Search if official web endpoint fails or returns empty
	a.mu.Lock()
	dev := a.dev
	a.mu.Unlock()
	if dev == nil {
		var err error
		dev, err = device.RegisterDevice()
		if err != nil {
			return nil, fmt.Errorf("无法连接片单来源: %w", err)
		}
		a.mu.Lock()
		a.dev = dev
		a.registered = true
		a.mu.Unlock()
	}

	const pageSize = 20
	if genre == "" || genre == "all" || genre == "hot" {
		group := "default"
		switch category {
		case "comic-drama", "hot-comic-drama":
			group = "comic_series"
		case "hot-ai-drama", "ai-drama":
			group = "ai_series"
		}
		result, err := hgapi.FetchCategoryLandpage(ctx, group, (page-1)*pageSize, pageSize, dev)
		if err == nil && result != nil && (len(result.Items) > 0 || page > 1) {
			return catalogPage(result.Items, result.HasMore, category), nil
		}
	}

	items, err := hgapi.Search(catalogKeyword(category, genre), page, dev)
	if err != nil {
		return nil, fmt.Errorf("片单加载失败: %w", err)
	}
	results := make([]hgapi.DramaCatalogItem, 0, len(items))
	for _, item := range items {
		results = append(results, hgapi.DramaCatalogItem{
			SeriesID: item.SeriesID, SeriesTitle: item.SeriesTitle, Cover: item.Cover,
			EpisodeCount: item.EpisodeCount, Description: item.Description,
		})
	}
	return catalogPage(results, len(items) >= pageSize, category), nil
}

func (a *App) GetMediaSources() (interface{}, error) {
	return map[string]interface{}{
		"success": true,
		"sources": []map[string]interface{}{{"id": "hongguo", "label": "牛油果短剧", "enabled": true}},
	}, nil
}

func (a *App) GetCustomSources() (interface{}, error) {
	return map[string]interface{}{"success": true, "sources": []interface{}{}}, nil
}
