package main

import (
	"testing"
)

func TestGetTalentAccessToken(t *testing.T) {
	got, err := GetTalentAccessToken()
	t.Log(got, err)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchDeleteSheetRecords(t *testing.T) {
	err := BatchDeleteSheetRecords(sheetAppID, bookListTableID, []string{"recggVJsus", "recue0Jgmx"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBatchAddSheetRecords(t *testing.T) {
	records := []map[string]interface{}{
		{
			"bookId":         "123456789",
			"书名":           "测试书名",
			"作者":           "测试作者",
			"价格":           10.99,
			"是否可读":       "否",
			"评分":           48,
			"评分（可视化）":   4,
			"阅读时长（秒）":   1800,
			"阅读时长格式化": "18时3分1秒",
			"分类":           []string{"测试分类1", "测试分类2"},
			"书架分类":       "心理学",
		},
	}

	err := BatchAddSheetRecords(sheetAppID, bookListTableID, records)
	if err != nil {
		t.Fatal(err)
	}
}
