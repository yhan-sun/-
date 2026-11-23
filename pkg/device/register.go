package device

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hongguo/pkg/crypto"
)

const registerURL = "https://log5-applog-lf.fqnovel.com/service/2/device_register/?tt_data=a&cronet_version=8d40f833_2026-03-03&ttnet_version=4.2.243.31-douyin&use_store_region_cookie=1"

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

type RegisterResponse struct {
	Code         int         `json:"code"`
	DeviceID     interface{} `json:"device_id"`
	DeviceIDStr  string      `json:"device_id_str"`
	InstallID    interface{} `json:"install_id"`
	InstallIDStr string      `json:"install_id_str"`
}

func parseID(val interface{}, preferred string) string {
	if preferred != "" {
		return preferred
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		if v > 0 {
			return fmt.Sprintf("%.0f", v)
		}
	}
	return ""
}

// RegisterDevice performs anonymous device registration with ByteDance applog servers.
func RegisterDevice() (*crypto.DeviceIdentity, error) {
	now := time.Now().UnixMilli()
	cdid := randomUUID()

	payloadMap := map[string]interface{}{
		"magic_tag": "ss_app_log",
		"header": map[string]interface{}{
			"display_name":           "红果免费短剧",
			"update_version_code":    73932,
			"manifest_version_code":  73932,
			"app_version_minor":      "7.3.9.32",
			"aid":                    8662,
			"channel":                "update_64",
			"package":                "com.phoenix.read",
			"app_version":            "7.3.9.32",
			"version_code":           73932,
			"sdk_version":            "3.7.0-rc.34-fanqie-xiaoshuo",
			"sdk_target_version":     29,
			"git_hash":               "3a88f4e",
			"os":                     "Android",
			"os_version":             "16",
			"os_api":                 36,
			"device_model":           "25053RT47C",
			"device_brand":           "Redmi",
			"device_manufacturer":    "Redmi",
			"cpu_abi":                "arm64-v8a",
			"release_build":          "b5e187f",
			"density_dpi":            520,
			"display_density":        "mdpi",
			"resolution":             "1280x2772",
			"language":               "zh",
			"timezone":               8,
			"access":                 "wifi",
			"not_request_sender":     0,
			"carrier":                "CHINA MOBILE",
			"mcc_mnc":                "46000",
			"rom":                    "eng.root.20231116.160340",
			"rom_version":            "W528JS release-keys",
			"cdid":                   cdid,
			"sig_hash":               "56a962410c494bbaf0b58dba20cae56f",
			"openudid":               randomHex(8),
			"clientudid":             randomUUID(),
			"ipv6_list":              []string{},
			"region":                 "CN",
			"tz_name":                "Asia/Shanghai",
			"tz_offset":              28800,
			"sim_region":             "cn",
			"sim_serial_number":      []string{},
			"oaid_may_support":       false,
			"req_id":                 randomUUID(),
			"device_platform":        "android",
			"custom": map[string]interface{}{
				"account_region_src": "fallback",
				"account_region":     "cn",
				"host_bit":           64,
				"dragon_device_type": "phone",
			},
			"pre_installed_channel":  "",
			"apk_first_install_time": now,
			"is_system_app":          0,
			"sdk_flavor":             "china",
			"guest_mode":             0,
		},
		"_gen_time": now,
	}

	rawJSON, err := json.Marshal(payloadMap)
	if err != nil {
		return nil, fmt.Errorf("marshal register json: %w", err)
	}

	compressed, err := crypto.GzipCompress(rawJSON)
	if err != nil {
		return nil, fmt.Errorf("gzip payload: %w", err)
	}

	data, err := crypto.TTEncrypt(compressed)
	if err != nil {
		return nil, fmt.Errorf("ttencrypt: %w", err)
	}

	m := md5.Sum(data)
	stub := strings.ToUpper(hex.EncodeToString(m[:]))

	req, err := http.NewRequest("POST", registerURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "com.phoenix.read/72232 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/octet-stream;tt-data=a")
	req.Header.Set("log-encode-type", "gzip")
	req.Header.Set("x-ss-dp", "8662")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", now))
	req.Header.Set("x-vc-bdturing-sdk-version", "4.0.3.cn")
	req.Header.Set("lc", "101")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("passport-sdk-version", "5051452")
	req.Header.Set("x-neptune", "-8|50:51:59:30:40:47:49:39")
	req.Header.Set("x-ss-stub", stub)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device_register: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var regResp RegisterResponse
	if err := json.Unmarshal(bodyBytes, &regResp); err != nil {
		return nil, fmt.Errorf("unmarshal response (%s): %w", string(bodyBytes), err)
	}

	deviceID := parseID(regResp.DeviceID, regResp.DeviceIDStr)
	installID := parseID(regResp.InstallID, regResp.InstallIDStr)

	if deviceID == "" || installID == "" {
		return nil, fmt.Errorf("invalid device registration response: %s", string(bodyBytes))
	}

	return &crypto.DeviceIdentity{
		DeviceID:    deviceID,
		IID:         installID,
		CDID:        cdid,
		VersionName: "7.2.2.32",
	}, nil
}
