package main

import (
	"encoding/json"
	"testing"

	"hongguo/pkg/storage"
)

func TestSettingsComponentResponse(t *testing.T) {
	legacy := storage.Settings{ProxyEnabled: true, ProxyPassword: "test-password", PictureInPictureMode: "native"}
	data, err := json.Marshal(settingsForUI(legacy))
	if err != nil {
		t.Fatal(err)
	}
	var view map[string]interface{}
	if err := json.Unmarshal(data, &view); err != nil {
		t.Fatal(err)
	}
	if view["success"] != true || view["picture_in_picture_mode"] != "native" ||
		view["proxy_enabled"] != false || view["proxy_password_set"] != true || view["proxy_password"] != "" {
		t.Fatal("settings did not match the component's response contract")
	}
}

func TestSaveSettingsRejectsUnavailableProxy(t *testing.T) {
	_, err := new(App).SaveSettings(map[string]interface{}{"proxy_enabled": true})
	if err == nil {
		t.Fatal("a transport that has not been configured must not report a successful enable")
	}
}
