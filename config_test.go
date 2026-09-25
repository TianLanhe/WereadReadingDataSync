package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppConfigRequiresFeishuIdentifiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.local.json")
	t.Setenv("WEREAD_SYNC_CONFIG", path)
	if _, err := loadAppConfig(); err == nil {
		t.Fatal("missing config should be reported")
	}
	data := []byte(`{"feishu":{"appId":"app","appSecret":"secret","baseAppId":"base","bookListTableId":"books","readTimeTableId":"time"}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Feishu.BaseAppID != "base" || config.Feishu.BookListTableID != "books" {
		t.Fatal("config identifiers were not loaded")
	}
}
