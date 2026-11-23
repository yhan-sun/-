package hgapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"hongguo/pkg/crypto"
)

const (
	LandpageAPI = "https://api5-normal-sinfonlinea.fqnovel.com/reading/distribution/category/landpage/v1/"
)

// DramaCatalogItem represents a rich drama item in catalog
type DramaCatalogItem struct {
	SeriesID         string   `json:"series_id"`
	SeriesTitle      string   `json:"series_title"`
	Cover            string   `json:"cover"`
	EpisodeCount     int      `json:"episode_count"`
	EpisodeLabel     string   `json:"episode_label"`
	CompletionStatus string   `json:"completion_status"` // "completed" or "ongoing"
	Heat             string   `json:"heat"`
	Score            string   `json:"score"`
	Tags             []string `json:"tags"`
	Description      string   `json:"description"`
}

type CatalogResult struct {
	Items      []DramaCatalogItem `json:"items"`
	HasMore    bool               `json:"has_more"`
	NextOffset int                `json:"next_offset"`
	SessionID  string             `json:"session_id"`
}

// FetchCategoryLandpage queries the official category landpage API for rich drama lists
func FetchCategoryLandpage(ctx context.Context, genre string, offset, limit int, identity *crypto.DeviceIdentity) (*CatalogResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	if genre == "" {
		genre = "default"
	}

	baseParams := []QueryParam{
		{"ac", "wifi"},
		{"channel", "update_64"},
		{"aid", "8662"},
		{"app_name", "novelread"},
		{"version_code", "72232"},
		{"version_name", "7.2.2.32"},
		{"device_platform", "android"},
		{"os", "android"},
		{"ssmix", "a"},
		{"device_type", "25053RT47C"},
		{"device_brand", "Redmi"},
		{"language", "zh"},
		{"os_api", "36"},
		{"os_version", "16"},
		{"manifest_version_code", "72232"},
		{"resolution", "1280*2772"},
		{"dpi", "520"},
		{"update_version_code", "72232"},
		{"host_abi", "arm64-v8a"},
		{"dragon_device_type", "phone"},
		{"pv_player", "72232"},
		{"compliance_status", "0"},
		{"need_personal_recommend", "1"},
		{"player_so_load", "1"},
		{"is_android_pad_screen", "0"},
		{"iid", identity.IID},
		{"device_id", identity.DeviceID},
		{"cdid", identity.CDID},
	}

	bodyMap := map[string]interface{}{
		"genre":     genre,
		"offset":    offset,
		"limit":     limit,
		"req_scene": "page_list",
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal landpage body: %w", err)
	}

	khronos := time.Now().Unix()
	ticketBase := time.Now().UnixMilli()

	var finalURL string
	var finalQuery string
	var requestTicket int64
	foundBranch0 := false

	for n := int64(0); n < 128; n++ {
		requestTicket = ticketBase + n
		params := append([]QueryParam{}, baseParams...)
		params = append(params,
			QueryParam{"ts", fmt.Sprintf("%d", khronos)},
			QueryParam{"_rticket", fmt.Sprintf("%d", requestTicket)},
		)
		q := BuildQueryString(params)
		u := LandpageAPI + "?" + q
		if crypto.BranchOf(u, bodyBytes, khronos) == 0 {
			finalURL = u
			finalQuery = q
			foundBranch0 = true
			break
		}
	}

	if !foundBranch0 {
		return nil, fmt.Errorf("failed to find branch 0 ticket after 128 tries")
	}

	randomGorgon := rand.Intn(65536)
	gorgon := crypto.XGorgon(finalQuery, bodyBytes, khronos, randomGorgon)
	helios := crypto.Helios(khronos)
	medusa := crypto.BuildMedusa(finalURL, bodyBytes, khronos, *identity)

	var be32 [4]byte
	binary.BigEndian.PutUint32(be32[:], uint32(khronos))
	ladon := base64.StdEncoding.EncodeToString(be32[:])

	var le32 [4]byte
	binary.LittleEndian.PutUint32(le32[:], uint32(khronos))
	argus := base64.StdEncoding.EncodeToString(le32[:])

	req, err := http.NewRequestWithContext(ctx, "POST", finalURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", SearchUA)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8,application/x-protobuf")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", requestTicket))
	req.Header.Set("x-tt-request-tag", "t=0;n=0")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-khronos", fmt.Sprintf("%d", khronos))
	req.Header.Set("x-ladon", ladon)
	req.Header.Set("x-argus", argus)
	req.Header.Set("x-gorgon", gorgon)
	req.Header.Set("x-helios", helios)
	req.Header.Set("x-medusa", medusa)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("landpage request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("landpage HTTP %d: %s", resp.StatusCode, string(b))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read landpage body: %w", err)
	}

	var raw struct {
		Code int `json:"code"`
		Data struct {
			HasMore    bool   `json:"has_more"`
			NextOffset int    `json:"next_offset"`
			SessionID  string `json:"session_id"`
			VideoData  []struct {
				SeriesID       interface{} `json:"series_id"`
				Title          string      `json:"title"`
				Cover          string      `json:"cover"`
				EpisodeCnt     interface{} `json:"episode_cnt"`
				Score          interface{} `json:"score"`
				PlayCnt        interface{} `json:"play_cnt"`
				VideoDesc      string      `json:"video_desc"`
				CategorySchema string      `json:"category_schema"`
				VideoDetail    struct {
					SeriesIntro  string      `json:"series_intro"`
					SeriesStatus interface{} `json:"series_status"`
				} `json:"video_detail"`
			} `json:"video_data"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(bytes.NewReader(respBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("unmarshal landpage response: %w", err)
	}

	var items []DramaCatalogItem
	seen := make(map[string]bool)

	for _, v := range raw.Data.VideoData {
		var sid string
		switch s := v.SeriesID.(type) {
		case json.Number:
			sid = s.String()
		case string:
			sid = s
		case float64:
			sid = fmt.Sprintf("%.0f", s)
		}
		if sid == "" || seen[sid] {
			continue
		}
		seen[sid] = true

		// Status
		status := "completed"
		switch st := v.VideoDetail.SeriesStatus.(type) {
		case json.Number:
			if n, err := st.Int64(); err == nil && n == 0 {
				status = "ongoing"
			}
		case float64:
			if int(st) == 0 {
				status = "ongoing"
			}
		}

		// Heat / play count
		heat := ""
		switch pc := v.PlayCnt.(type) {
		case json.Number:
			if n, err := pc.Float64(); err == nil {
				if n > 10000 {
					heat = fmt.Sprintf("热度 %.1f万", n/10000)
				} else if n > 0 {
					heat = fmt.Sprintf("热度 %.0f", n)
				}
			} else {
				heat = pc.String()
			}
		case string:
			heat = pc
		case float64:
			if pc > 10000 {
				heat = fmt.Sprintf("热度 %.1f万", pc/10000)
			} else if pc > 0 {
				heat = fmt.Sprintf("热度 %.0f", pc)
			}
		}
		if heat == "" {
			heat = "热度 9.8万"
		}

		// Score
		scoreStr := ""
		switch sc := v.Score.(type) {
		case json.Number:
			scoreStr = sc.String()
		case string:
			scoreStr = sc
		case float64:
			if sc > 0 {
				scoreStr = fmt.Sprintf("%.1f", sc)
			}
		}

		// Parse tags from category_schema
		tags := []string{}
		if v.CategorySchema != "" {
			var schemas []struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal([]byte(v.CategorySchema), &schemas); err == nil {
				for _, sc := range schemas {
					if sc.Name != "" && len(tags) < 3 {
						tags = append(tags, sc.Name)
					}
				}
			}
		}
		if len(tags) == 0 {
			if genre == "comic_series" {
				tags = []string{"动态漫", "热播"}
			} else {
				tags = []string{"热播", "独家"}
			}
		}

		intro := v.VideoDetail.SeriesIntro
		if intro == "" {
			intro = v.VideoDesc
		}

		var epCount int
		switch ec := v.EpisodeCnt.(type) {
		case json.Number:
			if n, err := ec.Int64(); err == nil {
				epCount = int(n)
			}
		case float64:
			epCount = int(ec)
		case int:
			epCount = ec
		}
		if epCount <= 0 {
			epCount = 1
		}

		items = append(items, DramaCatalogItem{
			SeriesID:         sid,
			SeriesTitle:      strings.TrimSpace(v.Title),
			Cover:            v.Cover,
			EpisodeCount:     epCount,
			EpisodeLabel:     fmt.Sprintf("%d 集", epCount),
			CompletionStatus: status,
			Heat:             heat,
			Score:            scoreStr,
			Tags:             tags,
			Description:      intro,
		})
	}

	return &CatalogResult{
		Items:      items,
		HasMore:    raw.Data.HasMore,
		NextOffset: raw.Data.NextOffset,
		SessionID:  raw.Data.SessionID,
	}, nil
}
