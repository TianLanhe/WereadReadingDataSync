package main

import (
	"os"
	"strings"
	"testing"
)

func TestGetTalentAccessToken(t *testing.T) {
	requireLiveTest(t)
	got, err := GetTalentAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Fatal("empty tenant access token")
	}
}

func TestBatchDeleteSheetRecords(t *testing.T) {
	requireLiveMutationTest(t)
	recordIDs := strings.FieldsFunc(os.Getenv("WEREAD_TEST_DELETE_RECORD_IDS"), func(r rune) bool { return r == ',' })
	if len(recordIDs) == 0 {
		t.Skip("set WEREAD_TEST_DELETE_RECORD_IDS to explicit record IDs")
	}
	config, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	err = BatchDeleteSheetRecords(config.Feishu.BaseAppID, config.Feishu.BookListTableID, recordIDs)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchAddSheetRecords(t *testing.T) {
	requireLiveMutationTest(t)
	config, err := loadAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	records := []map[string]interface{}{
		{
			"bookId":  "123456789",
			"书名":      "测试书名",
			"作者":      "测试作者",
			"价格":      10.99,
			"是否可读":    "否",
			"评分":      48,
			"评分（可视化）": 4,
			"阅读时长（秒）": 1800,
			"阅读时长格式化": "18时3分1秒",
			"分类":      []string{"测试分类1", "测试分类2"},
			"书架分类":    "心理学",
		},
	}

	err = BatchAddSheetRecords(config.Feishu.BaseAppID, config.Feishu.BookListTableID, records)
	if err != nil {
		t.Fatal(err)
	}
}
