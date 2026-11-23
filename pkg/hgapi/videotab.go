package hgapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hongguo/pkg/crypto"
	"hongguo/pkg/media/cenc"
)

const (
	VideoTabAPI = "https://api5-normal-sinfonlineb.fqnovel.com/reading/bookapi/video_tab/multi_video_model/v1/"
	VideoUA     = "com.phoenix.read/73932 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3; Cronet/TTNetVersion:04657795 2026-01-23 QuicVersion:c67e9834 2025-09-08)"
)

type PlayableStream struct {
	URL        string `json:"url"`
	ContentKey []byte `json:"content_key"`
	Codec      string `json:"codec"`
	Quality    string `json:"quality"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

// FetchVideoTabPlayModel queries ByteDance V5 video_tab API for playable video stream and decryption key
func FetchVideoTabPlayModel(ctx context.Context, vid string, dev *crypto.DeviceIdentity) (*PlayableStream, error) {
	if vid == "" {
		return nil, errors.New("empty vid")
	}
	if dev == nil {
		return nil, errors.New("nil device identity")
	}

	values := map[string]string{
		"aid":                       "8662",
		"app_name":                  "novelread",
		"version_code":              "74332",
		"version_name":              "7.4.3.32",
		"channel":                   "vivo_8662_64",
		"device_platform":           "android",
		"device_type":               "PGT-AN10",
		"os_version":                "12",
		"update_version_code":       "74332",
		"iid":                       dev.IID,
		"cdid":                      dev.CDID,
		"device_id":                 dev.DeviceID,
		"video_id":                  vid,
		"video_id_type":             "0",
		"device_level":              "1",
		"need_all_video_definition": "1",
		"need_mp4_align":            "0",
		"use_os_player":             "0",
		"video_platform":            "1024",
		"source":                    "0",
		"from_video_id":             "",
	}
	keys := []string{
		"aid", "app_name", "version_code", "version_name", "channel",
		"device_platform", "device_type", "os_version", "update_version_code",
		"iid", "cdid", "device_id", "video_id", "video_id_type", "device_level",
		"need_all_video_definition", "need_mp4_align", "use_os_player",
		"video_platform", "source", "from_video_id",
	}

	signedURL, headers, err := crypto.SignPlaybackV5(VideoTabAPI, values, keys, nil, time.Now())
	if err != nil {
		return nil, fmt.Errorf("sign video_tab request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", signedURL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", VideoUA)
	req.Header.Set("Accept", "application/json")

	// Direct transport to avoid proxy hang
	tr := &http.Transport{
		Proxy: nil,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   12 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("video_tab http error: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read video_tab body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("video_tab HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("EMPTY_RESPONSE from video_tab (status=%d, headers=%v, url=%s)", resp.StatusCode, resp.Header, signedURL)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &root); err != nil {
		return nil, fmt.Errorf("parse video_tab json: %w", err)
	}

	code, _ := root["code"].(float64)
	if code == 0 {
		code, _ = root["Code"].(float64)
	}
	if code != 0 {
		return nil, fmt.Errorf("video_tab business rejected: code %v", code)
	}

	dataMap, _ := root["data"].(map[string]interface{})
	if dataMap == nil {
		return nil, errors.New("missing data in video_tab response")
	}

	var entry map[string]interface{}
	if map1004, ok := dataMap["1004"].(map[string]interface{}); ok {
		entry, _ = map1004[vid].(map[string]interface{})
	}
	if entry == nil {
		if vmd, ok := dataMap["video_model_data"].(map[string]interface{}); ok {
			entry, _ = vmd[vid].(map[string]interface{})
		}
	}
	if entry == nil {
		entry, _ = dataMap[vid].(map[string]interface{})
	}
	if entry == nil {
		return nil, fmt.Errorf("vid %s not found in video_tab response", vid)
	}

	var model map[string]interface{}
	if vmRaw, ok := entry["video_model"]; ok {
		if vmStr, isStr := vmRaw.(string); isStr {
			_ = json.Unmarshal([]byte(vmStr), &model)
		} else if vmM, isM := vmRaw.(map[string]interface{}); isM {
			model = vmM
		}
	}
	if model == nil {
		model = entry
	}

	videoList, _ := model["video_list"].(map[string]interface{})
	if videoList == nil || len(videoList) == 0 {
		return nil, errors.New("no video_list in video_model")
	}

	keySeedStr, _ := model["key_seed"].(string)
	var keySeed []byte
	if keySeedStr != "" {
		keySeed, _ = base64.StdEncoding.DecodeString(keySeedStr)
	}

	// Prioritize video streams: h264 > hevc/bytevc1 > others
	type cand struct {
		item      map[string]interface{}
		codec     string
		codecRank int
		quality   string
		width     int
		height    int
	}

	var cands []cand
	for _, itRaw := range videoList {
		it, ok := itRaw.(map[string]interface{})
		if !ok {
			continue
		}
		codec, _ := it["codec_type"].(string)
		if codec == "" {
			if meta, ok := it["video_meta"].(map[string]interface{}); ok {
				codec, _ = meta["codec_type"].(string)
			}
		}
		codec = strings.ToLower(codec)
		codecRank := 3
		switch codec {
		case "h264", "avc", "avc1":
			codecRank = 0
		case "hevc", "h265", "hvc1", "hev1", "bytevc1":
			codecRank = 1
		case "bytevc2":
			codecRank = 2
		}

		quality, _ := it["definition"].(string)
		if quality == "" {
			quality, _ = it["quality_desc"].(string)
		}
		w, _ := it["vwidth"].(float64)
		h, _ := it["vheight"].(float64)

		cands = append(cands, cand{
			item:      it,
			codec:     codec,
			codecRank: codecRank,
			quality:   quality,
			width:     int(w),
			height:    int(h),
		})
	}

	// Pick lowest codecRank (0 = h264 preferred)
	var chosen *cand
	for i := range cands {
		c := &cands[i]
		if chosen == nil || c.codecRank < chosen.codecRank {
			chosen = c
		}
	}

	if chosen == nil {
		return nil, errors.New("no usable video streams in video_list")
	}

	item := chosen.item
	var spadeA string
	if s, ok := item["spade_a"].(string); ok && s != "" {
		spadeA = s
	} else if encInfo, ok := item["encrypt_info"].(map[string]interface{}); ok {
		spadeA, _ = encInfo["spade_a"].(string)
	}

	var contentKey []byte
	if spadeA != "" {
		ck, err := cenc.DeriveContentKey(spadeA)
		if err == nil {
			contentKey = ck
		}
	}

	// Extract candidate URL
	var targetURL string
	mainURL, _ := item["main_url"].(string)
	backupURL, _ := item["backup_url"].(string)

	urlList := []string{}
	if mainURL != "" {
		urlList = append(urlList, mainURL)
	}
	if backupURL != "" {
		urlList = append(urlList, backupURL)
	}
	if buList, ok := item["backup_url_list"].([]interface{}); ok {
		for _, u := range buList {
			if uStr, ok := u.(string); ok && uStr != "" {
				urlList = append(urlList, uStr)
			}
		}
	}

	for _, candURL := range urlList {
		if len(keySeed) > 0 {
			decrypted, err := cenc.DecryptSpadeURL(candURL, keySeed)
			if err == nil && decrypted != "" {
				targetURL = decrypted
				break
			}
		} else {
			targetURL = candURL
			break
		}
	}

	if targetURL == "" {
		return nil, errors.New("could not resolve valid media URL from video_tab")
	}

	return &PlayableStream{
		URL:        targetURL,
		ContentKey: contentKey,
		Codec:      chosen.codec,
		Quality:    chosen.quality,
		Width:      chosen.width,
		Height:     chosen.height,
	}, nil
}
