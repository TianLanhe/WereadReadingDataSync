package main

import (
	"time"
)

func CalculateReadTime(readingTimeFromWeread map[time.Time]int64, readingTimeFromSheet []*SheetRecord) ([]map[string]interface{}, []*SheetRecord, []string, error) {
	var toAdd []map[string]interface{}
	var toEdit []*SheetRecord
	var toDelete []string

	// 1. 先遍历 readingTimeFromSheet，找出不在 readingTimeFromWeread 存在的记录，添加到待删除列表中
	for _, record := range readingTimeFromSheet {
		dateUnixMilli := getInt64Value(record.Fields, "日期")
		date := time.UnixMilli(dateUnixMilli)
		if _, exists := readingTimeFromWeread[date]; !exists {
			toDelete = append(toDelete, record.RecordID)
		}
	}

	// 2. 再遍历 readingTimeFromWeread，找出不在 readingTimeFromSheet 存在的记录，添加到待添加列表中
	for date, readTime := range readingTimeFromWeread {
		dateUnixMilli1 := date.UnixMilli()
		found := false
		for _, record := range readingTimeFromSheet {
			dateUnixMilli2 := getInt64Value(record.Fields, "日期")
			if dateUnixMilli1 == dateUnixMilli2 {
				found = true
				break
			}
		}
		if !found {
			toAdd = append(toAdd, convertReadTimeToMap(date, readTime))
		}
	}

	// 3. 最后遍历 readingTimeFromSheet，找出值在 readingTimeFromWeread 存在差异的记录，添加到待编辑列表中
	for _, record := range readingTimeFromSheet {
		dateUnixMilli := getInt64Value(record.Fields, "日期")
		date := time.UnixMilli(dateUnixMilli)
		if readTime, exists := readingTimeFromWeread[date]; exists {
			readTimeFromSheet := getInt64Value(record.Fields, "当日阅读时长（秒）")
			if readTimeFromSheet != readTime {
				toEdit = append(toEdit, &SheetRecord{
					Fields:   convertReadTimeToMap(date, readTime),
					RecordID: record.RecordID,
				})
			}
		}
	}

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
