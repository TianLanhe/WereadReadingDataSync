package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestBookChangesRetainEveryComparedField(t *testing.T) {
	before := &Book{Title: "旧名", Author: "旧作者", Categories: []string{"旧分类"}}
	after := &Book{Title: "新名", Author: "新作者", Price: 1, CanRead: true, Score: 9, ReadTime: 60, ShelfName: "在读", Intro: "新简介", Words: 2, Progress: 0.2, FinishTime: 1000, Categories: []string{"新分类"}}
	changes := bookChanges(before, after)
	if len(changes) != 12 {
		t.Fatalf("got %d changes, want 12", len(changes))
	}
	var out bytes.Buffer
	r := NewConsoleReporter(&out, false, 80)
	r.Report(Event{Kind: EventChanges, Stage: "book", Changes: []RecordChange{{Kind: "update", Title: "旧名", Fields: changes}}})
	r.Close()
	for _, field := range []string{"书名", "作者", "价格", "是否可读", "阅读时长", "书架分类", "简介", "字数", "阅读进度", "读完时间", "分类"} {
		if !strings.Contains(out.String(), field) {
			t.Errorf("missing %s in %s", field, out.String())
		}
	}
	if strings.Contains(out.String(), "评分：") {
		t.Fatal("rating field must be hidden")
	}
}

func TestConsoleRatingOnlyChangesAreSummarizedAndNoItemLimit(t *testing.T) {
	var out bytes.Buffer
	r := NewConsoleReporter(&out, false, 80)
	var changes []RecordChange
	for i := 0; i < 25; i++ {
		changes = append(changes, RecordChange{Kind: "add", Title: strings.Repeat("书", i+1)})
	}
	changes = append(changes, RecordChange{Kind: "update", Title: "仅评分的书", Fields: []FieldChange{{Field: "Score", Before: 80.0, After: 80.1}}})
	r.Report(Event{Kind: EventChanges, Stage: "book", Changes: changes})
	r.Close()
	if !strings.Contains(out.String(), strings.Repeat("书", 25)) {
		t.Fatal("changes were truncated")
	}
	if strings.Contains(out.String(), "仅评分的书") {
		t.Fatal("rating-only book should not have an item")
	}
	if !strings.Contains(out.String(), "1 本仅更新评分") {
		t.Fatal(out.String())
	}
	if strings.Contains(out.String(), "\x1b") {
		t.Fatal("plain output contains terminal escapes")
	}
}
