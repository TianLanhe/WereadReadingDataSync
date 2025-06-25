package main

import (
	"sort"
	"time"
)

func CalculateReadTime(readingTimeFromWeread map[time.Time]int64, readingTimeFromSheet []*SheetRecord) ([]map[string]interface{}, []*SheetRecord, []string, error) {
	var toAdd []map[string]interface{}
	var toEdit []*SheetRecord
	var toDelete []string

	// 提取所有时间键
	keys := make([]time.Time, 0, len(readingTimeFromWeread))
	for k := range readingTimeFromWeread {
		keys = append(keys, k)
	}

	// 按时间从大到小排序
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].After(keys[j])
	})

	// 按排序后的顺序添加到toAdd
	for _, k := range keys {
		toAdd = append(toAdd, convertReadTimeToMap(k, readingTimeFromWeread[k]))
	}

	// TODO 补充编辑和删除的内容

	return toAdd, toEdit, toDelete, nil
}

func convertReadTimeToMap(date time.Time, readTime int64) map[string]interface{} {
	return map[string]interface{}{
		"日期":             date.UnixMilli(),
		"当日阅读时长（秒）": readTime,
		"当日阅读时长（时）": float64(readTime) / 3600,
		"当日阅读时长（分）": float64(readTime) / 60,
	}
}
