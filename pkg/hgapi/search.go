package hgapi

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hongguo/pkg/crypto"
)

const (
	SearchAPI = "https://api5-normal-sinfonlinea.fqnovel.com/reading/bookapi/search/tab/v"
	SearchUA  = "com.phoenix.read/72232 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3)"
)

type DramaItem struct {
	SeriesID     string `json:"series_id"`
	SeriesTitle  string `json:"series_title"`
	Cover        string `json:"cover"`
	EpisodeCount int    `json:"episode_count"`
	Description  string `json:"description"`
	URL          string `json:"url"`
}

type QueryParam struct {
	Key   string
	Value string
}

func encodePythonParam(s string) string {
	escaped := url.QueryEscape(s)
	// QueryEscape replaces spaces with '+', but standard python quote doesn't replace '!' '*' '(' ')'
	// In JS pythonUrlEncode:
	// encodeURIComponent(item).replace(/[!'()]/g, (ch) => `%${ch.charCodeAt(0).toString(16).toUpperCase()}`)
	// Let's match encodeURIComponent exactly:
	var buf strings.Builder
	for i := 0; i < len(escaped); i++ {
		ch := escaped[i]
		if ch == '+' {
			buf.WriteString("%20")
		} else if ch == '!' {
			buf.WriteString("%21")
		} else if ch == '\'' {
			buf.WriteString("%27")
		} else if ch == '(' {
			buf.WriteString("%28")
		} else if ch == ')' {
			buf.WriteString("%29")
		} else {
			buf.WriteByte(ch)
		}
	}
	return buf.String()
}

func BuildQueryString(params []QueryParam) string {
	var parts []string
	for _, p := range params {
		parts = append(parts, encodePythonParam(p.Key)+"="+encodePythonParam(p.Value))
	}
	return strings.Join(parts, "&")
}

func Search(keyword string, page int, identity *crypto.DeviceIdentity) ([]DramaItem, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * 20

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
		{"query", keyword},
		{"count", "20"},
		{"tab_type", "11"},
		{"bookshelf_search_plan", "4"},
		{"use_correct", "true"},
		{"user_is_login", "0"},
		{"offset", fmt.Sprintf("%d", offset)},
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
		u := SearchAPI + "?" + q
		if crypto.BranchOf(u, nil, khronos) == 0 {
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
	gorgon := crypto.XGorgon(finalQuery, nil, khronos, randomGorgon)
	helios := crypto.Helios(khronos)
	medusa := crypto.BuildMedusa(finalURL, nil, khronos, *identity)

	var be32 [4]byte
	binary.BigEndian.PutUint32(be32[:], uint32(khronos))
	ladon := base64.StdEncoding.EncodeToString(be32[:])

	var le32 [4]byte
	binary.LittleEndian.PutUint32(le32[:], uint32(khronos))
	argus := base64.StdEncoding.EncodeToString(le32[:])

	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", SearchUA)
	req.Header.Set("Accept", "application/json; charset=utf-8,application/x-protobuf")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", requestTicket))
	req.Header.Set("x-tt-request-tag", "t=0;n=0")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("passport-sdk-version", "50561")
	req.Header.Set("x-xs-from-web", "0")
	req.Header.Set("x-khronos", fmt.Sprintf("%d", khronos))
	req.Header.Set("x-ladon", ladon)
	req.Header.Set("x-argus", argus)
	req.Header.Set("x-gorgon", gorgon)
	req.Header.Set("x-helios", helios)
	req.Header.Set("x-medusa", medusa)
	req.Header.Set("x-tt-dt", "")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search http error %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search body: %w", err)
	}

	var raw map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("unmarshal search response: %w", err)
	}

	// Extract search_tabs
	var tabs []interface{}
	if t, ok := raw["search_tabs"].([]interface{}); ok {
		tabs = t
	} else if data, ok := raw["data"].(map[string]interface{}); ok {
		if t, ok := data["search_tabs"].([]interface{}); ok {
			tabs = t
		}
	}

	var results []DramaItem
	seen := make(map[string]bool)

	for _, tabVal := range tabs {
		tab, ok := tabVal.(map[string]interface{})
		if !ok {
			continue
		}
		var tabType int
		switch tt := tab["tab_type"].(type) {
		case json.Number:
			if n, err := tt.Int64(); err == nil {
				tabType = int(n)
			}
		case float64:
			tabType = int(tt)
		}
		if tabType != 11 {
			continue
		}

		cells, ok := tab["data"].([]interface{})
		if !ok {
			continue
		}

		for _, cellVal := range cells {
			cell, ok := cellVal.(map[string]interface{})
			if !ok {
				continue
			}

			videoList, ok := cell["video_data"].([]interface{})
			if !ok || len(videoList) == 0 {
				continue
			}

			for _, vVal := range videoList {
				video, ok := vVal.(map[string]interface{})
				if !ok {
					continue
				}

				var seriesID string
				switch sid := video["series_id"].(type) {
				case json.Number:
					seriesID = sid.String()
				case string:
					seriesID = sid
				case float64:
					seriesID = fmt.Sprintf("%.0f", sid)
				}

				if seriesID == "" || seen[seriesID] {
					continue
				}
				seen[seriesID] = true

				title, _ := video["title"].(string)
				if title == "" {
					title, _ = cell["cell_name"].(string)
				}
				cover, _ := video["cover"].(string)
				var epCount int
				switch cnt := video["episode_cnt"].(type) {
				case json.Number:
					if n, err := cnt.Int64(); err == nil {
						epCount = int(n)
					}
				case float64:
					epCount = int(cnt)
				}

				intro, _ := video["series_intro"].(string)
				if intro == "" {
					intro, _ = video["video_desc"].(string)
				}

				results = append(results, DramaItem{
					SeriesID:     seriesID,
					SeriesTitle:  title,
					Cover:        cover,
					EpisodeCount: epCount,
					Description:  intro,
					URL:          fmt.Sprintf("https://hongguoduanju.com/detail?series_id=%s", seriesID),
				})
			}
		}
	}

	return results, nil
}
