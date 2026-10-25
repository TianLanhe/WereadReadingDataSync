package main

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

const N = 5

type syncRunner struct {
	config   feishuConfig
	reporter Reporter
}

func updateReadTimeData(config feishuConfig) error {
	_, err := (&syncRunner{config: config}).readTime()
	return err
}
func updateBookListData(config feishuConfig) error {
	_, err := (&syncRunner{config: config}).books()
	return err
}

// parallelRead runs independent reads with a fixed bound and waits for every
// started request. A failure is never treated as a complete source snapshot.
func parallelRead(count, workers int, read func(int) error) error {
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	if count < workers {
		workers = count
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				mu.Lock()
				failed := firstErr != nil
				mu.Unlock()
				if failed {
					continue
				}
				if err := read(index); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for i := 0; i < count; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return firstErr
}
func (s *syncRunner) readTime() (stats SyncStats, resultErr error) {
	started := time.Now()
	report(s.reporter, Event{Kind: EventStart, Stage: "readTime"})
	defer func() {
		report(s.reporter, Event{Kind: EventFinish, Stage: "readTime", Stats: stats, Err: resultErr, Elapsed: time.Since(started)})
	}()
	yearly := make([]map[time.Time]int64, N)
	var records []*SheetRecord
	var progressMu sync.Mutex
	completed := 0
	err := parallelRead(2, 2, func(task int) error {
		if task == 1 {
			var err error
			records, err = ReadSheetRecords(s.config.BaseAppID, s.config.ReadTimeTableID)
			if err != nil {
				return fmt.Errorf("读取飞书阅读时长: %w", err)
			}
			report(s.reporter, Event{Kind: EventProgress, Stage: "readTime", Task: "existing", Done: len(records)})
			return nil
		}
		return parallelRead(N, 3, func(i int) error {
			_, days, err := GetYearReadingTime(i, s.reporter)
			if err != nil {
				return fmt.Errorf("读取第 %d 年阅读时长: %w", i+1, err)
			}
			yearly[i] = days
			progressMu.Lock()
			completed++
			report(s.reporter, Event{Kind: EventProgress, Stage: "readTime", Task: "years", Done: completed, Total: N})
			progressMu.Unlock()
			return nil
		})
	})
	if err != nil {
		return stats, err
	}
	source := make(map[time.Time]int64)
	for _, days := range yearly {
		for day, seconds := range days {
			if seconds != 0 {
				source[day] += seconds
			}
		}
	}
	add, update, remove, err := CalculateReadTime(source, records)
	if err != nil {
		return stats, err
	}
	// Stable write and display order makes repeated runs easy to compare.
	sort.Slice(add, func(i, j int) bool { return getInt64Value(add[i], "日期") < getInt64Value(add[j], "日期") })
	sort.Slice(update, func(i, j int) bool {
		return getInt64Value(update[i].Fields, "日期") < getInt64Value(update[j].Fields, "日期")
	})
	old := make(map[string]*SheetRecord)
	for _, r := range records {
		old[r.RecordID] = r
	}
	var changes []RecordChange
	for _, fields := range add {
		changes = append(changes, RecordChange{Kind: "add", Title: time.UnixMilli(getInt64Value(fields, "日期")).Format("2006-01-02"), Fields: []FieldChange{{Field: "ReadTime", Before: int64(0), After: getInt64Value(fields, "当日阅读时长（秒）")}}})
	}
	for _, r := range update {
		changes = append(changes, RecordChange{Kind: "update", Title: time.UnixMilli(getInt64Value(r.Fields, "日期")).Format("2006-01-02"), Fields: []FieldChange{{Field: "ReadTime", Before: getInt64Value(old[r.RecordID].Fields, "当日阅读时长（秒）"), After: getInt64Value(r.Fields, "当日阅读时长（秒）")}}})
	}
	for _, id := range remove {
		changes = append(changes, RecordChange{Kind: "delete", Title: time.UnixMilli(getInt64Value(old[id].Fields, "日期")).Format("2006-01-02")})
	}
	report(s.reporter, Event{Kind: EventPlan, Stage: "readTime", Stats: SyncStats{Add: len(add), Update: len(update), Delete: len(remove)}})
	report(s.reporter, Event{Kind: EventChanges, Stage: "readTime", Changes: changes})
	err = s.write("readTime", s.config.ReadTimeTableID, add, update, remove, &stats)
	return stats, err
}
func (s *syncRunner) books() (stats SyncStats, resultErr error) {
	started := time.Now()
	report(s.reporter, Event{Kind: EventStart, Stage: "book"})
	defer func() {
		report(s.reporter, Event{Kind: EventFinish, Stage: "book", Stats: stats, Err: resultErr, Elapsed: time.Since(started)})
	}()
	var shelf *BookShelfInfoResponse
	var finished map[string]*MineReadBook
	var existing []*SheetRecord
	err := parallelRead(3, 3, func(i int) error {
		switch i {
		case 0:
			var err error
			shelf, err = GetWeReadBookShelfInfo(s.reporter)
			if err != nil {
				return fmt.Errorf("读取书架: %w", err)
			}
			report(s.reporter, Event{Kind: EventProgress, Stage: "book", Task: "shelf", Done: len(shelf.Book)})
		case 1:
			var err error
			_, _, finished, err = GetMineReadBook(s.reporter)
			if err != nil {
				return fmt.Errorf("读取已读书籍: %w", err)
			}
			report(s.reporter, Event{Kind: EventProgress, Stage: "book", Task: "readBooks", Done: len(finished)})
		case 2:
			var err error
			existing, err = ReadSheetRecords(s.config.BaseAppID, s.config.BookListTableID)
			if err != nil {
				return fmt.Errorf("读取飞书书籍: %w", err)
			}
			report(s.reporter, Event{Kind: EventProgress, Stage: "book", Task: "existing", Done: len(existing)})
		}
		return nil
	})
	if err != nil {
		return stats, err
	}
	seen := make(map[string]bool)
	var ids []string
	for _, b := range shelf.Book {
		if b != nil && b.BookId != "" && !seen[b.BookId] {
			seen[b.BookId] = true
			ids = append(ids, b.BookId)
		}
	}
	for id := range finished {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	report(s.reporter, Event{Kind: EventProgress, Stage: "book", Task: "details", Total: len(ids)})
	details, err := BatchGetBookDetail(ids, s.reporter)
	if err != nil {
		return stats, err
	}
	plan, err := makeBookPlan(shelf, details, finished, existing)
	if err != nil {
		return stats, err
	}
	sort.Slice(plan.Adds, func(i, j int) bool { return plan.Adds[i].BookId < plan.Adds[j].BookId })
	sort.Slice(plan.Updates, func(i, j int) bool { return plan.Updates[i].Book.BookId < plan.Updates[j].Book.BookId })
	for _, err := range plan.Warnings {
		report(s.reporter, Event{Kind: EventWarning, Stage: "book", Err: err})
	}
	report(s.reporter, Event{Kind: EventPlan, Stage: "book", Stats: SyncStats{Add: len(plan.Adds), Update: len(plan.Updates), Delete: len(plan.Deletes)}})
	report(s.reporter, Event{Kind: EventChanges, Stage: "book", Changes: plan.Changes})
	uniqueCovers := make(map[string]bool)
	for _, url := range plan.Covers {
		uniqueCovers[url] = true
	}
	if len(uniqueCovers) > 0 {
		report(s.reporter, Event{Kind: EventProgress, Stage: "book", Task: "covers", Total: len(uniqueCovers)})
	}
	manager := newCoverUploadManager(coverUploadOptions{Total: len(uniqueCovers), Workers: 5, Interval: 205 * time.Millisecond, Wait: maxPicUploadWaitTime, Reporter: s.reporter, Upload: func(ctx context.Context, name string, data []byte, gate func() error) (string, error) {
		return uploadMediaToBaseWithGate(ctx, name, data, s.config.BaseAppID, gate)
	}})
	add, update := materializeBookPlan(plan, manager, s.reporter)
	manager.Close()
	err = s.write("book", s.config.BookListTableID, add, update, plan.Deletes, &stats)
	return stats, err
}
func (s *syncRunner) write(stage, table string, add []map[string]interface{}, update []*SheetRecord, remove []string, stats *SyncStats) error {
	const batchSize = 200
	for start := 0; start < len(add); start += batchSize {
		end := start + batchSize
		if end > len(add) {
			end = len(add)
		}
		if err := BatchAddSheetRecords(s.config.BaseAppID, table, add[start:end]); err != nil {
			return fmt.Errorf("新增记录失败（该批结果未确认）: %w", err)
		}
		stats.Add += end - start
		report(s.reporter, Event{Kind: EventProgress, Stage: stage, Task: "add", Done: stats.Add, Total: len(add)})
	}
	for start := 0; start < len(update); start += batchSize {
		end := start + batchSize
		if end > len(update) {
			end = len(update)
		}
		if err := BatchUpdateSheetRecords(s.config.BaseAppID, table, update[start:end]); err != nil {
			return fmt.Errorf("更新记录失败（该批结果未确认）: %w", err)
		}
		stats.Update += end - start
		report(s.reporter, Event{Kind: EventProgress, Stage: stage, Task: "update", Done: stats.Update, Total: len(update)})
	}
	for start := 0; start < len(remove); start += batchSize {
		end := start + batchSize
		if end > len(remove) {
			end = len(remove)
		}
		if err := BatchDeleteSheetRecords(s.config.BaseAppID, table, remove[start:end]); err != nil {
			return fmt.Errorf("删除记录失败（该批结果未确认）: %w", err)
		}
		stats.Delete += end - start
		report(s.reporter, Event{Kind: EventProgress, Stage: stage, Task: "delete", Done: stats.Delete, Total: len(remove)})
	}
	return nil
}
