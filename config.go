package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

type feishuConfig struct {
	AppID           string `json:"appId"`
	AppSecret       string `json:"appSecret"`
	BaseAppID       string `json:"baseAppId"`
	BookListTableID string `json:"bookListTableId"`
	ReadTimeTableID string `json:"readTimeTableId"`
}

type appConfig struct {
	Feishu feishuConfig `json:"feishu"`
}

func appConfigPath() string {
	if path := os.Getenv("WEREAD_SYNC_CONFIG"); path != "" {
		return path
	}
	return "config.local.json"
}

func loadAppConfig() (appConfig, error) {
	path := appConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return appConfig{}, fmt.Errorf("未找到 %s：请复制 config.example.json 并填写飞书配置", path)
		}
		return appConfig{}, fmt.Errorf("读取配置失败: %w", err)
	}
	var config appConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return appConfig{}, fmt.Errorf("配置 JSON 格式无效: %w", err)
	}
	f := config.Feishu
	if strings.TrimSpace(f.AppID) == "" || strings.TrimSpace(f.AppSecret) == "" || strings.TrimSpace(f.BaseAppID) == "" || strings.TrimSpace(f.BookListTableID) == "" || strings.TrimSpace(f.ReadTimeTableID) == "" {
		return appConfig{}, errors.New("config.local.json 缺少飞书 appId、appSecret、baseAppId 或表格 ID")
	}
	return config, nil
}
