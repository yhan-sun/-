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
	VideoDetailAPI = "https://api5-normal-sinfonlinea.fqnovel.com/novel/player/video_detail/v1/"
)

// FetchVideoDetail fetches complete series metadata and full episode video_list via official API
func FetchVideoDetail(ctx context.Context, seriesID string, identity *crypto.DeviceIdentity) (*SeriesDetail, error) {
	seriesID = strings.TrimSpace(seriesID)
	if seriesID == "" {
		return nil, fmt.Errorf("empty series_id")
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
		"series_id":                 seriesID,
		"caller_scene":              "single_col",
		"detail_page_version":       "1",
		"disable_digg_stat":         0,
		"disable_video_relate_book": 0,
		"from_video_id":             "0",
		"need_all_video_definition": 0,
		"need_mp4_align":            0,
		"screen_width_px":           "1078",
		"source":                    "single_col",
		"use_os_player":             0,
		"use_server_dns":            0,
	}

	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal video_detail body: %w", err)
	}

	khronos := time.Now().Unix()
	ticketBase := time.Now().UnixMilli()

	var finalURL, finalQuery string
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
		u := VideoDetailAPI + "?" + q
		if crypto.BranchOf(u, bodyBytes, khronos) == 0 {
			finalURL = u
			finalQuery = q
			foundBranch0 = true
			break
		}
	}

	if !foundBranch0 {
		return nil, fmt.Errorf("branch 0 not found for video_detail")
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

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video_detail request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("video_detail HTTP %d: %s", resp.StatusCode, string(b))
	}

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read video_detail body: %w", err)
	}

	var raw struct {
		Code int `json:"code"`
		Data struct {
			VideoData struct {
				SeriesID    interface{} `json:"series_id"`
				SeriesTitle string      `json:"series_title"`
				SeriesCover string      `json:"series_cover"`
				SeriesIntro string      `json:"series_intro"`
				EpisodeCnt  int         `json:"episode_cnt"`
				VideoList   []struct {
					Vid      interface{} `json:"vid"`
					VidIndex int         `json:"vid_index"`
					Title    string      `json:"title"`
					Cover    string      `json:"cover"`
					Duration int         `json:"duration"`
				} `json:"video_list"`
			} `json:"video_data"`
		} `json:"data"`
	}

	if err := json.Unmarshal(respBytes, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal video_detail: %w", err)
	}

	vd := raw.Data.VideoData
	var sid string
	switch s := vd.SeriesID.(type) {
	case string:
		sid = s
	case float64:
		sid = fmt.Sprintf("%.0f", s)
	}
	if sid == "" {
		sid = seriesID
	}

	episodes := make([]Episode, len(vd.VideoList))
	for i, v := range vd.VideoList {
		var epVid string
		switch ev := v.Vid.(type) {
		case string:
			epVid = ev
		case float64:
			epVid = fmt.Sprintf("%.0f", ev)
		}

		epIndex := v.VidIndex
		if epIndex <= 0 {
			epIndex = i + 1
		}

		epCover := v.Cover
		if epCover == "" {
			epCover = vd.SeriesCover
		}

		episodes[i] = Episode{
			Vid:          epVid,
			VidIndex:     epIndex,
			Title:        fmt.Sprintf("第%d集", epIndex),
			SeriesID:     sid,
			SeriesTitle:  vd.SeriesTitle,
			Cover:        epCover,
			WebAvailable: true,
			Locked:       false,
		}
	}

	total := vd.EpisodeCnt
	if total <= 0 || total < len(episodes) {
		total = len(episodes)
	}

	return &SeriesDetail{
		SeriesID:              sid,
		SeriesTitle:           vd.SeriesTitle,
		Cover:                 vd.SeriesCover,
		Total:                 total,
		WebAccessibleEpisodes: len(episodes),
		Episodes:              episodes,
		Source:                "app_video_detail",
	}, nil
}
