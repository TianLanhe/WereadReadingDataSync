package main

import (
	"fmt"
	"time"
)

func CalculateReadTime(source map[time.Time]int64, records []*SheetRecord) ([]map[string]interface{}, []*SheetRecord, []string, error) {
	var add []map[string]interface{}
	var update []*SheetRecord
	var remove []string
	existing := make(map[int64]bool, len(records))
	byDay := make(map[int64]int64, len(source))
	for day, seconds := range source {
		byDay[day.UnixMilli()] = seconds
	}
	for _, r := range records {
		if r == nil || r.RecordID == "" {
			return nil, nil, nil, fmt.Errorf("阅读时长记录缺少标识")
		}
		stamp := getInt64Value(r.Fields, "日期")
		if stamp == 0 {
			return nil, nil, nil, fmt.Errorf("阅读时长记录缺少有效日期")
		}
		existing[stamp] = true
		seconds, exists := byDay[stamp]
		if !exists {
			remove = append(remove, r.RecordID)
		} else if getInt64Value(r.Fields, "当日阅读时长（秒）") != seconds {
			update = append(update, &SheetRecord{RecordID: r.RecordID, Fields: convertReadTimeToMap(time.UnixMilli(stamp), seconds)})
		}
	}
	for day, seconds := range source {
		if !existing[day.UnixMilli()] {
			add = append(add, convertReadTimeToMap(day, seconds))
		}
	}
	return add, update, remove, nil
}
func convertReadTimeToMap(date time.Time, seconds int64) map[string]interface{} {
	return map[string]interface{}{"日期": date.UnixMilli(), "当日阅读时长（秒）": seconds, "当日阅读时长（时）": float64(seconds) / 3600, "当日阅读时长（分）": float64(seconds) / 60}
}
