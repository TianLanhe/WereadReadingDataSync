package main

import "fmt"

type bookUpdate struct {
	Book     *Book
	RecordID string
}
type bookSyncPlan struct {
	Adds     []*Book
	Updates  []bookUpdate
	Deletes  []string
	Changes  []RecordChange
	Covers   map[string]string // book ID -> URL, only missing attachments
	Warnings []error
}

func attachmentPresent(value any) bool {
	switch v := value.(type) {
	case []any:
		return len(v) > 0
	case []map[string]any:
		return len(v) > 0
	case []map[string]string:
		return len(v) > 0
	}
	return false
}
func makeBookPlan(shelf *BookShelfInfoResponse, details map[string]*BookDetailResponse, finished map[string]*MineReadBook, records []*SheetRecord) (bookSyncPlan, error) {
	plan := bookSyncPlan{Covers: make(map[string]string)}
	if shelf == nil {
		return plan, fmt.Errorf("书架数据为空")
	}
	for _, r := range records {
		if r == nil {
			return plan, fmt.Errorf("飞书书籍包含空记录")
		}
	}
	for _, b := range shelf.Book {
		if b == nil {
			return plan, fmt.Errorf("书架包含空书籍")
		}
	}
	for _, p := range shelf.BookProgress {
		if p == nil {
			return plan, fmt.Errorf("书架包含空进度")
		}
	}
	for _, a := range shelf.Archive {
		if a == nil {
			return plan, fmt.Errorf("书架包含空分类")
		}
	}
	for _, b := range finished {
		if b == nil {
			return plan, fmt.Errorf("已读列表包含空书籍")
		}
		if b.FinishTime == 0 {
			plan.Warnings = append(plan.Warnings, fmt.Errorf("《%s》已标为读完，但缺少读完时间", b.Title))
		}
	}

	source := TransferShelfResponseToBook(shelf, details, finished)
	for id, b := range TransferShelfResponseToBook(TransferMineReadBookToShelfResponse(finished), details, finished) {
		if _, exists := source[id]; !exists {
			source[id] = b
		}
	}
	existing := make(map[string]*Book)
	recordByID := make(map[string]*SheetRecord)
	books, _ := TransferSheetRecordToBook(records)
	for _, b := range books {
		existing[b.BookId] = b
	}
	for _, r := range records {
		if r == nil {
			continue
		}
		id := getStringValue(r.Fields, "bookId")
		if id != "" {
			if r.RecordID == "" {
				return plan, fmt.Errorf("书籍《%s》缺少记录标识", getStringValue(r.Fields, "书名"))
			}
			recordByID[id] = r
		}
	}
	for id, b := range source {
		old, exists := existing[id]
		if !exists {
			plan.Adds = append(plan.Adds, b)
			plan.Changes = append(plan.Changes, RecordChange{Kind: "add", Title: b.Title})
			if b.Cover != "" {
				plan.Covers[id] = b.Cover
			}
			continue
		}
		fields := bookChanges(old, b)
		record := recordByID[id]
		if record == nil {
			return plan, fmt.Errorf("书籍《%s》无法对应飞书记录", b.Title)
		}
		if b.Cover != "" && !attachmentPresent(record.Fields["封面"]) {
			plan.Covers[id] = b.Cover
			fields = append(fields, FieldChange{Field: "Cover", Before: "缺失", After: "补充封面"})
		}
		if len(fields) > 0 {
			plan.Updates = append(plan.Updates, bookUpdate{b, record.RecordID})
			plan.Changes = append(plan.Changes, RecordChange{Kind: "update", Title: b.Title, Fields: fields})
		}
	}
	for id, b := range existing {
		if _, exists := source[id]; !exists {
			record := recordByID[id]
			if record == nil {
				return plan, fmt.Errorf("书籍《%s》无法对应飞书记录", b.Title)
			}
			plan.Deletes = append(plan.Deletes, record.RecordID)
			plan.Changes = append(plan.Changes, RecordChange{Kind: "delete", Title: b.Title})
		}
	}
	return plan, nil
}
func materializeBookPlan(plan bookSyncPlan, m *CoverUploadManager, r Reporter) ([]map[string]interface{}, []*SheetRecord) {
	for _, url := range plan.Covers {
		m.StartUpload(url)
	}
	tokens := make(map[string]string)
	get := func(b *Book) string {
		url, needed := plan.Covers[b.BookId]
		if !needed {
			return ""
		}
		if token, exists := tokens[url]; exists {
			return token
		}
		token, err := m.GetResult(url)
		if err != nil {
			report(r, Event{Kind: EventWarning, Stage: "book", Err: fmt.Errorf("《%s》封面处理失败，保留原附件或留空：%w", b.Title, err)})
		}
		tokens[url] = token
		return token
	}
	var add []map[string]interface{}
	var update []*SheetRecord
	for _, b := range plan.Adds {
		add = append(add, convertBookToMap(b, get(b)))
	}
	for _, u := range plan.Updates {
		update = append(update, &SheetRecord{RecordID: u.RecordID, Fields: convertBookToMap(u.Book, get(u.Book))})
	}
	return add, update
}
